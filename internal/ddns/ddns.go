package ddns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"netlite/internal/model"
	"netlite/internal/store"
	"os"
	"strings"
	"sync"
	"time"
)

type Status struct {
	At      time.Time `json:"at"`
	IP      string    `json:"ip"`
	OK      bool      `json:"ok"`
	Message string    `json:"message"`
}
type Provider interface {
	Update(context.Context, model.DDNS, string) error
}
type Cloudflare struct {
	Client *http.Client
	Base   string
}

func (p Cloudflare) Update(ctx context.Context, d model.DDNS, ip string) error {
	token := os.Getenv(d.TokenEnv)
	if token == "" {
		return fmt.Errorf("credential environment %s missing", d.TokenEnv)
	}
	endpoint := p.Base + "/zones/" + d.ZoneID + "/dns_records/" + d.RecordID
	call := func(method string, body []byte) (json.RawMessage, error) {
		req, _ := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, e := p.Client.Do(req)
		if e != nil {
			return nil, fmt.Errorf("Cloudflare connection failed")
		}
		defer res.Body.Close()
		var envelope struct {
			Success bool
			Result  json.RawMessage
		}
		if e = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope); e != nil {
			return nil, fmt.Errorf("invalid Cloudflare response")
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 || !envelope.Success {
			return nil, fmt.Errorf("Cloudflare request rejected (HTTP %d)", res.StatusCode)
		}
		return envelope.Result, nil
	}
	result, e := call("GET", nil)
	if e != nil {
		return e
	}
	var record struct{ Name, Type, Content string }
	if e = json.Unmarshal(result, &record); e != nil {
		return e
	}
	if record.Name != d.Name || record.Type != d.Type {
		return fmt.Errorf("record ID does not match configured name/type")
	}
	if record.Content == ip {
		return nil
	}
	// PATCH content only, preserving TTL, proxy setting and other record attributes.
	body, _ := json.Marshal(map[string]string{"content": ip})
	_, e = call("PATCH", body)
	return e
}

type Worker struct {
	Store    *store.Store
	Provider Provider
	Client   *http.Client
	mu       sync.Mutex
	next     map[string]time.Time
}

func New(s *store.Store) *Worker {
	client := &http.Client{Timeout: 20 * time.Second}
	return &Worker{Store: s, Provider: Cloudflare{client, "https://api.cloudflare.com/client/v4"}, Client: client, next: map[string]time.Time{}}
}
func (w *Worker) Update(ctx context.Context, d model.DDNS) Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	status := Status{At: time.Now().UTC()}
	ip := d.IP
	if ip == "" {
		endpoint := "https://api4.ipify.org"
		if d.Type == "AAAA" {
			endpoint = "https://api6.ipify.org"
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
		res, e := w.Client.Do(req)
		if e != nil {
			status.Message = "public IP lookup failed"
		} else {
			b, e := io.ReadAll(io.LimitReader(res.Body, 128))
			res.Body.Close()
			if e != nil || res.StatusCode != 200 {
				status.Message = "public IP lookup failed"
			} else {
				ip = strings.TrimSpace(string(b))
			}
		}
	}
	if status.Message == "" {
		parsed := net.ParseIP(ip)
		if parsed == nil || (d.Type == "A") != (parsed.To4() != nil) {
			status.Message = "invalid public IP/address family"
		} else {
			status.IP = ip
			if e := w.Provider.Update(ctx, d, ip); e != nil {
				status.Message = e.Error()
			} else {
				status.OK = true
				status.Message = "record synchronized"
			}
		}
	}
	_ = w.Store.Status(d.ID, status)
	w.next[d.ID] = time.Now().Add(time.Duration(d.Interval) * time.Second)
	return status
}
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		cfg, e := w.Store.Load()
		if e == nil {
			for _, d := range cfg.DDNS {
				w.mu.Lock()
				due := time.Now().After(w.next[d.ID])
				w.mu.Unlock()
				if d.Enabled && due {
					w.Update(ctx, d)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
