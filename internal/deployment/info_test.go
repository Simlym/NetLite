package deployment

import (
	"netlite/internal/model"
	"testing"
)

func TestDeploymentInformation(t *testing.T) {
	i, err := Parse("bridge", "443:18443,8443:8443", false)
	if err != nil || !i.MappingsKnown || len(i.Mappings) != 2 || i.Mappings[0].PublicPort != 443 || i.Mappings[0].ListenPort != 18443 {
		t.Fatal(i, err)
	}
	for _, text := range []string{"443", "0:18443", "443:65536", "443:x", "443:18443,443:8443"} {
		if _, err := Parse("bridge", text, false); err == nil {
			t.Fatalf("accepted invalid mapping %q", text)
		}
	}
	i, err = Parse("bridge", "", false)
	if err != nil || i.MappingsKnown {
		t.Fatal("missing mapping must remain unknown", i, err)
	}
	if _, err := Parse("guessed", "", false); err == nil {
		t.Fatal("accepted unknown network mode")
	}
}

func TestFnOSPolicyDoesNotLeakIntoDomainModel(t *testing.T) {
	cfg := model.Config{Routes: []model.Route{{ID: "app", Domain: "app.example.com", Scheme: "http", Port: 80, Upstream: "http://127.0.0.1:8080", Enabled: true}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"host", "native", "bridge", "unknown"} {
		i, err := Parse(mode, "", false)
		if err != nil || i.Validate(cfg) != nil {
			t.Fatal(mode, i, err)
		}
	}
	i, err := Parse("host", "", true)
	if err != nil || i.Mode != "fnos" || i.Validate(cfg) == nil {
		t.Fatal("gateway must preserve fnOS port restrictions", i, err)
	}
	cfg.Routes[0].Port = 18080
	if err := i.Validate(cfg); err != nil {
		t.Fatal(err)
	}
}
