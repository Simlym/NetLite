package model

import (
	"strings"
	"testing"
)

func TestValidateBoundary(t *testing.T) {
	base := Config{Certificates: []Certificate{{ID: "cert", Domains: []string{"app.example.com"}, Mode: "internal"}}, Routes: []Route{{ID: "app", Domain: "app.example.com", Port: 18443, Scheme: "https", Upstream: "https://localhost:8080", CertificateID: "cert", Enabled: true}}}
	if e := base.Validate(18780, 20190); e != nil {
		t.Fatal(e)
	}
	cases := []struct {
		name string
		edit func(*Config)
	}{
		{"reserved port", func(c *Config) { c.Routes[0].Port = 18780 }},
		{"privileged port", func(c *Config) { c.Routes[0].Port = 443 }},
		{"uncovered TLS", func(c *Config) { c.Routes[0].Domain = "other.example.com" }},
		{"upstream path", func(c *Config) { c.Routes[0].Upstream = "http://localhost/admin" }},
		{"wildcard without DNS", func(c *Config) { c.Certificates[0].Domains = []string{"*.example.com"} }},
		{"scheme collision", func(c *Config) {
			c.Routes = append(c.Routes, Route{ID: "b", Domain: "other.example.com", Scheme: "http", Port: 18443, Upstream: "http://localhost", Enabled: true})
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c := base
			c.Routes = append([]Route{}, base.Routes...)
			c.Certificates = append([]Certificate{}, base.Certificates...)
			tt.edit(&c)
			if c.Validate(18780, 20190) == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}
func TestWildcardDepth(t *testing.T) {
	if !Covers("*.example.com", "app.example.com") || Covers("*.example.com", "example.com") || Covers("*.example.com", "a.b.example.com") {
		t.Fatal("wildcard coverage incorrect")
	}
}

func TestCertificateValidationReportsAllAffectedRoutes(t *testing.T) {
	cfg := Config{
		Certificates: []Certificate{{ID: "other", Domains: []string{"other.example.com"}, Mode: "internal"}},
		Routes: []Route{
			{ID: "photos", Name: "家庭相册", Domain: "photos.example.com", Port: 18443, Scheme: "https", Upstream: "http://localhost:8080", CertificateID: "deleted", Enabled: true},
			{ID: "downloads", Domain: "downloads.example.com", Port: 18443, Scheme: "https", Upstream: "http://localhost:8080", CertificateID: "deleted", Enabled: false},
			{ID: "mismatch", Name: "证书不匹配", Domain: "app.example.com", Port: 18443, Scheme: "https", Upstream: "http://localhost:8080", CertificateID: "other", Enabled: true},
			{ID: "unselected", Name: "尚未选择证书", Domain: "new.example.com", Port: 18443, Scheme: "https", Upstream: "http://localhost:8080", Enabled: true},
			{ID: "valid", Name: "正常规则", Domain: "other.example.com", Port: 18443, Scheme: "https", Upstream: "http://localhost:8080", CertificateID: "other", Enabled: true},
			{ID: "plain", Name: "普通 HTTP", Domain: "plain.example.com", Port: 18080, Scheme: "http", Upstream: "http://localhost:8080", CertificateID: "deleted", Enabled: true},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("accepted invalid certificate references")
	}
	for _, want := range []string{
		"「家庭相册」（ID：photos，photos.example.com:18443，已启用）",
		"「downloads」（ID：downloads，downloads.example.com:18443，已停用）",
		"引用的证书（ID：deleted）不存在或已被删除",
		"「证书不匹配」（ID：mismatch，app.example.com:18443，已启用）：所选证书（other.example.com）不覆盖该域名",
		"「尚未选择证书」（ID：unselected，new.example.com:18443，已启用）：未选择证书",
		"已停用的 HTTPS 规则也需要有效证书",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not identify %q", err, want)
		}
	}
	for _, valid := range []string{"正常规则", "普通 HTTP"} {
		if strings.Contains(err.Error(), valid) {
			t.Errorf("error incorrectly includes %q: %v", valid, err)
		}
	}
}
