package proxy

import (
	"encoding/json"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigurationRedactsCheckpointAndKeepsDraft(t *testing.T) {
	c := New("unused", t.TempDir(), "127.0.0.1:20190")
	cfg := model.Config{Certificates: []model.Certificate{{Domains: []string{"*.example.com"}, Mode: "dns", Provider: "cloudflare", TokenEnv: "CF_TOKEN"}}}
	view, err := c.Configuration(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if view["applied"] != nil {
		t.Fatal("unexpected checkpoint")
	}
	err = os.WriteFile(filepath.Join(c.Data, "applied.json"), []byte(`{"apps":{"tls":{"api_token":"live-secret","nested":[{"password":"hidden"}]}}}`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	view, err = c.Configuration(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(view)
	text := string(b)
	if strings.Contains(text, "live-secret") || strings.Contains(text, "hidden") {
		t.Fatal("credential leaked")
	}
	if !strings.Contains(text, "{env.CF_TOKEN}") || !strings.Contains(text, "[REDACTED]") {
		t.Fatal("incorrect config view")
	}
}
