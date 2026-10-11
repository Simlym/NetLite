package model

import "testing"

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
