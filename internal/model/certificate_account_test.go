package model

import "testing"

func TestCertificateAccountChoice(t *testing.T) {
	base := Certificate{ID: "cert", Domains: []string{"*.example.com"}, Mode: "dns", AccountID: "account"}
	if e := (Config{Certificates: []Certificate{base}}).Validate(); e != nil {
		t.Fatal(e)
	}
	for _, c := range []Certificate{
		{ID: "cert", Domains: base.Domains, Mode: "dns", AccountID: "account", Provider: "cloudflare", TokenEnv: "CF_TOKEN"},
		{ID: "cert", Domains: base.Domains, Mode: "dns", AccountID: "bad.id"},
		{ID: "cert", Domains: []string{"example.com"}, Mode: "acme", AccountID: "account"},
	} {
		if (Config{Certificates: []Certificate{c}}).Validate() == nil {
			t.Fatal("ambiguous/invalid account accepted")
		}
	}
	base.AccountID = ""
	base.Provider = "cloudflare"
	base.TokenEnv = "CF_TOKEN"
	if e := (Config{Certificates: []Certificate{base}}).Validate(); e != nil {
		t.Fatal("legacy config no longer valid", e)
	}
}
