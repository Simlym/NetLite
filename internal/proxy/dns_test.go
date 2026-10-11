package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedDNSCredentials(t *testing.T) {
	for _, provider := range DNSProviders {
		t.Run(provider, func(t *testing.T) {
			c := New("unused", t.TempDir(), "127.0.0.1:20190")
			c.ResolveAccount = func(id string) (model.DNSCredential, error) {
				return model.DNSCredential{Provider: provider, ID: "12345", Secret: "private-test-token"}, nil
			}
			cfg := model.Config{Certificates: []model.Certificate{{ID: "cert", Mode: "dns", Domains: []string{"*.example.com"}, AccountID: "account", Staging: true}}}
			b, e := c.Render(cfg)
			if e != nil {
				t.Fatal(e)
			}
			if bytes.Contains(b, []byte("private-test-token")) || !bytes.Contains(b, []byte("{netlite.dns.account.")) {
				t.Fatal("draft leaked or lost account reference")
			}
			runtime, e := c.runtimeConfig(b)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Contains(runtime, []byte("private-test-token")) || bytes.Contains(runtime, []byte("{netlite.dns.")) {
				t.Fatal("runtime did not resolve credentials")
			}
			var view any
			json.Unmarshal(b, &view)
			vb, _ := json.Marshal(redact(view))
			if !bytes.Contains(vb, []byte("{netlite.dns.")) || bytes.Contains(vb, []byte("private-test-token")) {
				t.Fatal("preview lost reference or leaked secret")
			}
			os.WriteFile(filepath.Join(c.Data, "applied.json"), b, 0600)
			if !c.AccountReferenced("account") || c.AccountReferenced("acc") {
				t.Fatal("applied reference guard failed")
			}
			c.ResolveAccount = func(string) (model.DNSCredential, error) {
				return model.DNSCredential{Provider: "different", Secret: "private-test-token"}, nil
			}
			if _, e = c.runtimeConfig(b); e == nil {
				t.Fatal("changed provider accepted for checkpoint")
			}
		})
	}
}
func TestSharedCredentialsRealCaddy(t *testing.T) {
	binary := os.Getenv("NETLITE_TEST_CADDY")
	if binary == "" {
		t.Skip("real Caddy binary not configured")
	}
	binary, _ = filepath.Abs(binary)
	for _, provider := range DNSProviders {
		t.Run(provider, func(t *testing.T) {
			var received []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received, _ = io.ReadAll(r.Body) }))
			defer server.Close()
			c := New(binary, t.TempDir(), strings.TrimPrefix(server.URL, "http://"))
			c.ResolveAccount = func(string) (model.DNSCredential, error) {
				return model.DNSCredential{Provider: provider, ID: "12345", Secret: strings.Repeat("a", 40)}, nil
			}
			cfg := model.Config{Certificates: []model.Certificate{{ID: "dns", Domains: []string{"*.example.com"}, Mode: "dns", AccountID: "account", Staging: true}}}
			if e := c.Apply(context.Background(), cfg); e != nil {
				t.Fatal(e)
			}
			checkpoint, e := os.ReadFile(filepath.Join(c.Data, "applied.json"))
			if e != nil {
				t.Fatal(e)
			}
			if bytes.Contains(checkpoint, []byte(strings.Repeat("a", 40))) || !bytes.Contains(received, []byte(strings.Repeat("a", 40))) {
				t.Fatal("checkpoint/transport credential boundary failed")
			}
			if _, e = c.runtimeConfig(checkpoint); e != nil {
				t.Fatal("restart reference resolution failed", e)
			}
		})
	}
}
