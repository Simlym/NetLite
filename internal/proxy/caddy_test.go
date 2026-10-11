package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderIndependentPortsAndTLS(t *testing.T) {
	c := New("caddy", t.TempDir(), "127.0.0.1:20190")
	cfg := model.Config{Routes: []model.Route{{Domain: "plain.example.com", Scheme: "http", Port: 18081, Upstream: "http://127.0.0.1:8080", Enabled: true}, {Domain: "secure.example.com", Scheme: "https", Port: 18444, Upstream: "https://localhost:8081", Enabled: true}}, Certificates: []model.Certificate{{Domains: []string{"secure.example.com"}, Mode: "dns", Provider: "cloudflare", TokenEnv: "CF_TOKEN", Staging: true}}}
	b, e := c.Render(cfg)
	if e != nil {
		t.Fatal(e)
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil {
		t.Fatal("invalid JSON")
	}
	for _, s := range []string{":18081", ":18444", "dns", "{env.CF_TOKEN}", "acme-staging", "server_name"} {
		if !strings.Contains(string(b), s) {
			t.Fatalf("missing %s", s)
		}
	}
	if strings.Contains(string(b), "insecure_skip_verify") {
		t.Fatal("unsafe upstream TLS")
	}
}
func TestRejectOccupiedAdmin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("existing service must never be called") }))
	defer server.Close()
	c := New("does-not-exist", t.TempDir(), strings.TrimPrefix(server.URL, "http://"))
	if e := c.Run(context.Background()); e == nil || !strings.Contains(e.Error(), "occupied") {
		t.Fatalf("expected port conflict, got %v", e)
	}
}
func TestValidateRealCaddy(t *testing.T) {
	binary := os.Getenv("NETLITE_TEST_CADDY")
	if binary == "" {
		t.Skip("set NETLITE_TEST_CADDY to validate against real Caddy")
	}
	binary, _ = filepath.Abs(binary)
	c := New(binary, t.TempDir(), "127.0.0.1:20190")
	cfg := model.Config{Certificates: []model.Certificate{{ID: "internal", Domains: []string{"localhost"}, Mode: "internal"}}, Routes: []model.Route{{ID: "http", Domain: "plain.localhost", Scheme: "http", Port: 18081, Upstream: "http://localhost:8080", Enabled: true}, {ID: "https", Domain: "localhost", Scheme: "https", Port: 18444, Upstream: "https://localhost:8081", Enabled: true}}}
	if e := c.Validate(context.Background(), cfg); e != nil {
		t.Fatal(e)
	}
	cfg.Certificates = []model.Certificate{{ID: "dns", Domains: []string{"*.example.com"}, Mode: "dns", Provider: "cloudflare", TokenEnv: "NETLITE_CF_TEST", Staging: true}}
	mods, _ := c.Modules(context.Background())
	for _, m := range mods {
		if m == "dns.providers.cloudflare" {
			t.Setenv("NETLITE_CF_TEST", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			if e := c.Validate(context.Background(), cfg); e != nil {
				t.Fatal(e)
			}
			return
		}
	}
}
