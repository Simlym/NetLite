package certificates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"netlite/internal/dnsprovider"
	"strings"
	"sync"
	"time"

	"github.com/mholt/acmez/v3/acme"
)

type HTTPChallenge struct {
	Address string
	mu      sync.Mutex
	server  *http.Server
	tokens  map[string]acme.Challenge
}

func NewHTTPChallenge(address string) *HTTPChallenge {
	return &HTTPChallenge{Address: address, tokens: map[string]acme.Challenge{}}
}
func (h *HTTPChallenge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	host := r.Host
	if v, _, err := net.SplitHostPort(host); err == nil {
		host = v
	}
	for _, c := range h.tokens {
		if r.URL.Path == c.HTTP01ResourcePath() && strings.EqualFold(host, c.Identifier.Value) {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(c.KeyAuthorization))
			return
		}
	}
	http.NotFound(w, r)
}
func (h *HTTPChallenge) Present(ctx context.Context, c acme.Challenge) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.server == nil {
		listener, err := net.Listen("tcp", h.Address)
		if err != nil {
			return fmt.Errorf("HTTP-01 验证端口被占用")
		}
		h.server = &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 10 * time.Second}
		srv := h.server
		go func() { _ = srv.Serve(listener) }()
	}
	h.tokens[c.Token] = c
	return nil
}
func (h *HTTPChallenge) CleanUp(_ context.Context, c acme.Challenge) error {
	h.mu.Lock()
	delete(h.tokens, c.Token)
	var srv *http.Server
	if len(h.tokens) == 0 {
		srv = h.server
		h.server = nil
	}
	h.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
	return nil
}
func (h *HTTPChallenge) Close() {
	h.mu.Lock()
	srv := h.server
	h.server = nil
	clear(h.tokens)
	h.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

type txtJournal struct {
	AccountID string
	Record    dnsprovider.TXTRecord
}
type dnsSolver struct {
	manager             *Manager
	accountID, provider string
	client              *dnsprovider.Client
	scope               string
}

func (s *dnsSolver) journalID(c acme.Challenge) string {
	h := sha256.Sum256([]byte(s.scope + ":" + c.Token))
	return hex.EncodeToString(h[:])
}
func (m *Manager) saveTXT(id string, r txtJournal) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = m.db.DB.Exec("INSERT INTO certificate_txt VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", id, r.AccountID, string(b))
	return err
}
func (s *dnsSolver) Present(ctx context.Context, c acme.Challenge) error {
	name := strings.TrimSuffix(c.DNS01TXTRecordName(), ".")
	lookup := s.manager.lookupCNAME
	if lookup == nil {
		lookup = net.DefaultResolver.LookupCNAME
	}
	canonical, err := lookup(ctx, name)
	if err == nil {
		name = strings.TrimSuffix(canonical, ".")
	} else {
		dnsErr, ok := err.(*net.DNSError)
		if !ok || !dnsErr.IsNotFound {
			return fmt.Errorf("cannot resolve DNS challenge name")
		}
	}
	value := c.DNS01KeyAuthorization()
	rows, err := s.client.ListTXT(ctx, name)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Value == value {
			return s.manager.saveTXT(s.journalID(c), txtJournal{AccountID: s.accountID, Record: r})
		}
	}
	// Persist the exact-value intent before the remote write, so a crash between
	// creation and receipt of its ID can be recovered without deleting an RRset.
	intent := txtJournal{AccountID: s.accountID, Record: dnsprovider.TXTRecord{Provider: s.provider, Name: name, Value: value, Owned: true}}
	if err = s.manager.saveTXT(s.journalID(c), intent); err != nil {
		return err
	}
	record, err := s.client.PresentTXT(ctx, name, value)
	if err != nil {
		return err
	}
	intent.Record = record
	return s.manager.saveTXT(s.journalID(c), intent)
}
func (s *dnsSolver) Wait(ctx context.Context, c acme.Challenge) error {
	var b string
	if err := s.manager.db.DB.QueryRow("SELECT body FROM certificate_txt WHERE id=?", s.journalID(c)).Scan(&b); err != nil {
		return err
	}
	var journal txtJournal
	if err := json.Unmarshal([]byte(b), &journal); err != nil {
		return err
	}
	lookup := s.manager.lookupTXT
	if lookup == nil {
		lookup = net.DefaultResolver.LookupTXT
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		values, err := lookup(wait, journal.Record.Name)
		if err == nil {
			for _, v := range values {
				if v == journal.Record.Value {
					return nil
				}
			}
		}
		select {
		case <-wait.Done():
			return fmt.Errorf("DNS TXT 传播等待超时或已取消")
		case <-ticker.C:
		}
	}
}
func (s *dnsSolver) CleanUp(_ context.Context, c acme.Challenge) error {
	// Cleanup gets a short independent deadline even when issuance is canceled.
	// Unfinished cleanup remains in the journal for retry after restart.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.manager.cleanTXT(ctx, s.journalID(c), s.client)
}
func (m *Manager) cleanTXT(ctx context.Context, id string, client *dnsprovider.Client) error {
	var b string
	err := m.db.DB.QueryRow("SELECT body FROM certificate_txt WHERE id=?", id).Scan(&b)
	if err != nil {
		return err
	}
	var journal txtJournal
	if err = json.Unmarshal([]byte(b), &journal); err != nil {
		return err
	}
	if err = client.CleanTXT(ctx, journal.Record); err != nil {
		return err
	}
	_, err = m.db.DB.Exec("DELETE FROM certificate_txt WHERE id=?", id)
	return err
}
func (m *Manager) recoverTXT(ctx context.Context) error {
	if !m.operationMu.TryLock() {
		return nil
	}
	defer m.operationMu.Unlock()
	return m.recoverTXTLocked(ctx, "")
}
func (m *Manager) recoverTXTLocked(ctx context.Context, accountID string) error {
	rows, err := m.db.DB.Query("SELECT id,account_id FROM certificate_txt")
	if err != nil {
		return err
	}
	type entry struct{ id, account string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.account); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if accountID != "" && e.account != accountID {
			continue
		}
		if m.ResolveAccount == nil {
			return fmt.Errorf("DNS account resolver unavailable")
		}
		a, err := m.ResolveAccount(e.account)
		if err != nil {
			return err
		}
		if err = m.cleanTXT(ctx, e.id, m.DNSClient(a)); err != nil {
			return err
		}
	}
	return nil
}
