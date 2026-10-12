// Package certificates owns TLS material and its lifecycle independently of proxy engines.
package certificates

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"netlite/internal/dnsprovider"
	"netlite/internal/model"
	"netlite/internal/store"
	"path/filepath"
	"sync"
	"time"
)

type State struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	Working     bool      `json:"working"`
	NotAfter    time.Time `json:"not_after"`
	RenewAt     time.Time `json:"renew_at"`
	LastIssued  time.Time `json:"last_issued"`
	NextAttempt time.Time `json:"next_attempt"`
	Attempts    int       `json:"attempts"`
	LastError   string    `json:"last_error,omitempty"`
	ReloadError string    `json:"reload_error,omitempty"`
}
type record struct {
	Policy     model.Certificate
	State      State
	Generation string
}
type Manager struct {
	db             *store.Store
	dir            string
	ctx            context.Context
	mu, rootMu     sync.Mutex
	operationMu sync.Mutex
	queued         map[string]bool
	queue          chan model.Certificate
	now            func() time.Time
	Applied        func() (model.Config, error)
	Reload         func(context.Context) error
	ResolveAccount func(string) (model.DNSCredential, error)
	DNSClient      func(model.DNSCredential) *dnsprovider.Client
	Issue          func(context.Context, model.Certificate) (Bundle, error)
	HTTP           *HTTPChallenge
	acmeDirectory  string
	acmeHTTP       *http.Client
	lookupTXT      func(context.Context, string) ([]string, error)
	lookupCNAME    func(context.Context, string) (string, error)
}

func Open(ctx context.Context, db *store.Store, dir, httpListen string) (*Manager, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = privateDir(dir); err != nil {
		return nil, err
	}
	if _, err = db.DB.Exec(`CREATE TABLE IF NOT EXISTS certificate_state(id TEXT PRIMARY KEY,body TEXT NOT NULL); CREATE TABLE IF NOT EXISTS certificate_txt(id TEXT PRIMARY KEY,account_id TEXT NOT NULL,body TEXT NOT NULL);`); err != nil {
		return nil, err
	}
	m := &Manager{db: db, dir: dir, ctx: ctx, queue: make(chan model.Certificate, 128), queued: map[string]bool{}, now: time.Now, DNSClient: dnsprovider.New, HTTP: NewHTTPChallenge(httpListen)}
	m.Issue = m.issue
	return m, nil
}
func (m *Manager) load(c model.Certificate) (record, error) {
	var b string
	err := m.db.DB.QueryRow("SELECT body FROM certificate_state WHERE id=?", PolicyKey(c)).Scan(&b)
	if err == sql.ErrNoRows {
		return record{Policy: c, State: State{ID: c.ID, Status: "pending"}}, nil
	}
	if err != nil {
		return record{}, err
	}
	var r record
	err = json.Unmarshal([]byte(b), &r)
	return r, err
}
func (m *Manager) save(r record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = m.db.DB.Exec("INSERT INTO certificate_state VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", PolicyKey(r.Policy), string(b))
	return err
}
func (m *Manager) Request(c model.Certificate, force bool) error {
	if err := (model.Config{Certificates: []model.Certificate{c}}).Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := PolicyKey(c)
	if m.queued[key] {
		return nil
	}
	if m.ctx.Err() != nil {
		return m.ctx.Err()
	}
	r, err := m.load(c)
	if err != nil {
		return err
	}
	if force && r.State.LastIssued.After(m.now().Add(-time.Minute)) {
		return fmt.Errorf("证书最近已签发，请稍后再试")
	}
	if r.State.NextAttempt.After(m.now()) {
		return fmt.Errorf("证书申请处于退避期，请等待下一次重试")
	}
	if !force && r.Generation != "" && r.State.RenewAt.After(m.now()) {
		return nil
	}
	previous := r
	r.Policy = c
	r.State.Working = true
	r.State.Status = "queued"
	r.State.LastError = ""
	if err = m.save(r); err != nil {
		return err
	}
	m.queued[key] = true
	select {
	case m.queue <- c:
		return nil
	default:
		delete(m.queued, key)
		_ = m.save(previous)
		return fmt.Errorf("certificate queue full")
	}
}
func (m *Manager) policies() ([]model.Certificate, error) {
	cfg, err := m.db.Load()
	if err != nil {
		return nil, err
	}
	all := map[string]model.Certificate{}
	for _, c := range cfg.Certificates {
		all[PolicyKey(c)] = c
	}
	if m.Applied != nil {
		applied, err := m.Applied()
		if err != nil {
			return nil, err
		}
		for _, c := range applied.Certificates {
			if _, ok := all[PolicyKey(c)]; !ok {
				all[PolicyKey(c)] = c
			}
		}
	}
	out := []model.Certificate{}
	for _, c := range all {
		out = append(out, c)
	}
	return out, nil
}
func (m *Manager) Sweep() error {
	all, err := m.policies()
	if err != nil {
		return err
	}
	for _, c := range all {
		r, err := m.load(c)
		if err != nil {
			return err
		}
		if r.State.ReloadError != "" && m.Reload != nil {
			ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
			err = m.Reload(ctx)
			cancel()
			if err == nil {
				// Clear only this field: a concurrent issuance may have published a new generation.
				if _, err = m.db.DB.Exec("UPDATE certificate_state SET body=json_remove(body,'$.State.reload_error') WHERE id=?", PolicyKey(c)); err != nil {
					return err
				}
			}
		}
		if r.State.NextAttempt.After(m.now()) {
			continue
		}
		if err = m.Request(c, false); err != nil && m.ctx.Err() == nil {
			log.Printf("certificate %s scheduling failed", c.ID)
		}
	}
	return nil
}
func (m *Manager) Run(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case c := <-m.queue:
				if ctx.Err() != nil {
					return
				}
				m.execute(c)
			}
		}
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		cleanup, cancel := context.WithTimeout(ctx, 30*time.Second)
		_ = m.recoverTXT(cleanup)
		cancel()
		if err := m.Sweep(); err != nil {
			log.Print("certificate scheduling unavailable")
		}
		select {
		case <-ctx.Done():
			<-done
			m.HTTP.Close()
			return
		case <-ticker.C:
		}
	}
}
func (m *Manager) execute(c model.Certificate) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	key := PolicyKey(c)
	defer func() { m.mu.Lock(); delete(m.queued, key); m.mu.Unlock() }()
	r, err := m.load(c)
	if err != nil {
		return
	}
	r.State.Status = "issuing"
	if r.Generation != "" {
		r.State.Status = "renewing"
	}
	r.State.Working = true
	if m.save(r) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, 15*time.Minute)
	defer cancel()
	var b Bundle
	if c.Mode == "dns" {
		cleanup, stop := context.WithTimeout(ctx, 30*time.Second)
		err = m.recoverTXTLocked(cleanup, c.AccountID)
		stop()
	}
	if err == nil {
		b, err = m.Issue(ctx, c)
	}
	if err == nil {
		var expires, renew time.Time
		gen, leaf, e := m.publish(c, b)
		err = e
		if err == nil {
			expires = leaf.NotAfter
			window := time.Duration(c.RenewBeforeDays) * 24 * time.Hour
			if window == 0 {
				window = 30 * 24 * time.Hour
				if third := leaf.NotAfter.Sub(leaf.NotBefore) / 3; third < window {
					window = third
				}
			}
			if window >= leaf.NotAfter.Sub(m.now()) {
				window = leaf.NotAfter.Sub(m.now()) / 3
			}
			renew = expires.Add(-window)
			r.Generation = gen
			r.State.NotAfter = expires
			r.State.RenewAt = renew
			r.State.LastIssued = m.now()
			r.State.LastError = ""
			r.State.Attempts = 0
			r.State.NextAttempt = time.Time{}
			r.State.Status = "ready"
		}
	}
	r.State.Working = false
	if err != nil {
		r.State.Status = "failed"
		r.State.LastError = "签发或续期失败，请检查验证方式、账号权限、DNS 传播及网络"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			r.State.LastError = "证书验证超时或已取消"
		}
		r.State.Attempts++
		shift := r.State.Attempts - 1
		if shift > 5 {
			shift = 5
		}
		r.State.NextAttempt = m.now().Add(time.Duration(1<<shift) * 15 * time.Minute)
	}
	if err = m.save(r); err != nil {
		log.Printf("certificate %s result checkpoint failed", c.ID)
		return
	}
	if r.State.Status == "ready" && m.Reload != nil {
		reloadCtx, stop := context.WithTimeout(m.ctx, 30*time.Second)
		err = m.Reload(reloadCtx)
		stop()
		if err != nil {
			r.State.ReloadError = "证书已更新，代理重载失败，将自动重试"
			_ = m.save(r)
		}
	}
}
func (m *Manager) Status(c model.Certificate) (State, error) {
	r, err := m.load(c)
	if err != nil {
		return State{}, err
	}
	s := r.State
	m.mu.Lock()
	s.Working = m.queued[PolicyKey(c)]
	m.mu.Unlock()
	if !s.Working && (s.Status == "issuing" || s.Status == "queued" || s.Status == "renewing") {
		s.Status = "pending"
	}
	if !s.Working && r.Generation != "" && !s.NotAfter.After(m.now()) {
		s.Status = "expired"
	}
	return s, nil
}
func (m *Manager) AccountReferenced(id string) bool {
	var n int
	if err := m.db.DB.QueryRow("SELECT count(*) FROM certificate_txt WHERE account_id=?", id).Scan(&n); err != nil {
		return true
	}
	if n > 0 {
		return true
	}
	rows, err := m.db.DB.Query("SELECT body FROM certificate_state")
	if err != nil {
		return true
	}
	defer rows.Close()
	for rows.Next() {
		var b string
		if rows.Scan(&b) != nil {
			return true
		}
		var r record
		if json.Unmarshal([]byte(b), &r) != nil {
			return true
		}
		if r.Policy.AccountID == id && r.State.Working {
			return true
		}
	}
	return rows.Err() != nil
}
