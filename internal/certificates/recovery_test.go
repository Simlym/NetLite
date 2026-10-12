package certificates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"netlite/internal/dnsprovider"
	"netlite/internal/model"
	"strings"
	"testing"
	"time"
)

func TestTXTIntentRecoveryAndChangedRecordProtection(t *testing.T) {
	m := testManager(t)
	values := map[string]string{"foreign": "other-challenge", "owned": "our-challenge"}
	a := model.DNSCredential{Provider: "cloudflare", Secret: "dummy"}
	client := func(a model.DNSCredential) *dnsprovider.Client {
		return dnsprovider.NewWithHTTP(a, &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			var result any
			if r.URL.Path == "/client/v4/zones" {
				result = []any{map[string]string{"id": "zone", "name": "example.com", "status": "active"}}
			} else if r.Method == "DELETE" {
				if !strings.HasSuffix(r.URL.Path, "/owned") { t.Fatal("deleted foreign record") }
				delete(values, "owned")
				result = map[string]string{"id": "owned"}
			} else {
				records := []any{}
				for id, value := range values { records = append(records, map[string]string{"id": id, "name": "_acme-challenge.example.com", "type": "TXT", "content": value}) }
				result = records
			}
			b, _ := json.Marshal(map[string]any{"success": true, "result": result})
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b)))}, nil
		})})
	}
	m.ResolveAccount = func(string) (model.DNSCredential, error) { return a, nil }
	m.DNSClient = client
	journal := txtJournal{AccountID: "account", Record: dnsprovider.TXTRecord{Provider: "cloudflare", Name: "_acme-challenge.example.com", Value: "our-challenge", Owned: true}}
	if err := m.saveTXT("interrupted", journal); err != nil { t.Fatal(err) }
	if !m.AccountReferenced("account") { t.Fatal("pending cleanup did not protect account") }
	if err := m.recoverTXT(context.Background()); err != nil { t.Fatal(err) }
	if len(values) != 1 || values["foreign"] != "other-challenge" || m.AccountReferenced("account") { t.Fatal("intent recovery failed") }
	values["owned"] = "changed"
	journal.Record.ID = "owned"
	if err := m.saveTXT("changed", journal); err != nil { t.Fatal(err) }
	if err := m.recoverTXT(context.Background()); err == nil { t.Fatal("changed record cleanup accepted") }
	if values["owned"] != "changed" || !m.AccountReferenced("account") { t.Fatal("failed cleanup lost its journal") }
}

func TestReloadRetryCannotOverwriteNewMaterial(t *testing.T) {
	m := testManager(t)
	c := policy()
	execute(t, m, c)
	r, _ := m.load(c)
	r.State.ReloadError = "retry"
	m.save(r)
	cfg, _ := m.db.Load()
	cfg.Certificates = []model.Certificate{c}
	m.db.Save(cfg)
	newGeneration := "abcdef0123456789abcdef0123456789"
	m.Reload = func(context.Context) error {
		latest, err := m.load(c)
		if err != nil { return err }
		latest.Generation = newGeneration
		latest.State.LastIssued = time.Now().Add(time.Minute)
		return m.save(latest)
	}
	if err := m.Sweep(); err != nil { t.Fatal(err) }
	latest, err := m.load(c)
	if err != nil || latest.Generation != newGeneration || latest.State.ReloadError != "" { t.Fatal("retry overwrote a newly published generation") }
}

func TestManagerCancellationStopsActiveIssuance(t *testing.T) {
	m := testManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	c := policy()
	started := make(chan struct{})
	m.Issue = func(ctx context.Context, _ model.Certificate) (Bundle, error) { close(started); <-ctx.Done(); return Bundle{}, ctx.Err() }
	if err := m.Request(c, true); err != nil { t.Fatal(err) }
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	select { case <-started: case <-time.After(time.Second): t.Fatal("worker did not start") }
	cancel()
	select { case <-done: case <-time.After(2*time.Second): t.Fatal("canceled issuance retained worker") }
	state, err := m.Status(c)
	if err != nil || state.Working || state.Status != "failed" { t.Fatal(fmt.Sprintf("unexpected canceled state: %+v %v", state, err)) }
}
