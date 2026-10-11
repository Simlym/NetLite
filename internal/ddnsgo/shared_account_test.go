package ddnsgo

import (
	"context"
	"netlite/internal/model"
	"netlite/internal/store"
	"path/filepath"
	"testing"
)

func TestCertificateAccountReferences(t *testing.T) {
	dir := t.TempDir()
	db, e := store.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.DB.Close()
	s, e := OpenService(context.Background(), db, "", filepath.Join(dir, "private"))
	if e != nil {
		t.Fatal(e)
	}
	h := s.Router()
	a := accountInput{DNSAccount: model.DNSAccount{ID: "shared", Name: "Shared", Provider: "dnspod", CredentialID: "12345"}, Secret: "dummy-token"}
	request(t, h, "PUT", "/accounts/shared", a, 200)
	cfg, _ := db.Load()
	cfg.Certificates = []model.Certificate{{ID: "cert", Mode: "dns", Domains: []string{"*.example.com"}, AccountID: "shared"}}
	if e = s.ValidateCertificates(cfg); e != nil {
		t.Fatal(e)
	}
	cfg, e = db.Save(cfg)
	if e != nil {
		t.Fatal(e)
	}
	request(t, h, "DELETE", "/accounts/shared", nil, 409)
	a.Provider = "tencentcloud"
	request(t, h, "PUT", "/accounts/shared", a, 409)
	a.Provider = "dnspod"
	a.Secret = "rotated-token"
	request(t, h, "PUT", "/accounts/shared", a, 200)
	resolved, e := s.ResolveAccount("shared")
	if e != nil || resolved.Secret != "rotated-token" {
		t.Fatal("shared credential rotation failed")
	}
	cfg.Certificates = nil
	cfg, e = db.Save(cfg)
	if e != nil {
		t.Fatal(e)
	}
	s.AppliedAccountReference = func(string) bool { return true }
	request(t, h, "DELETE", "/accounts/shared", nil, 409)
	s.AppliedAccountReference = func(string) bool { return false }
	request(t, h, "DELETE", "/accounts/shared", nil, 200)
	cfg.Certificates = []model.Certificate{{ID: "cert", Mode: "dns", AccountID: "shared"}}
	if s.ValidateCertificates(cfg) == nil {
		t.Fatal("dangling certificate account accepted")
	}
}
