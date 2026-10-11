package store

import (
	"errors"
	"netlite/internal/model"
	"path/filepath"
	"testing"
)

func TestPersistenceAndRevisionConflict(t *testing.T) {
	p := filepath.Join(t.TempDir(), "test.db")
	s, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.Load()
	if e != nil {
		t.Fatal(e)
	}
	c.Routes = []model.Route{{ID: "app", Name: "相册 · 家庭照片", Domain: "photos.example.com", Scheme: "http", Port: 18080, Upstream: "http://127.0.0.1:8080", Enabled: true}}
	saved, e := s.Save(c)
	if e != nil || saved.Revision != 1 {
		t.Fatalf("save: %v", e)
	}
	if _, e = s.Save(c); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update accepted")
	}
	s.DB.Close()
	s, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	c, e = s.Load()
	if e != nil || c.Revision != 1 || len(c.Routes) != 1 || c.Routes[0].Name != "相册 · 家庭照片" {
		t.Fatal("not persisted")
	}
}

func TestLegacyRouteWithoutName(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.DB.Close()
	_, e = s.DB.Exec(`UPDATE config SET body='{"revision":0,"routes":[{"id":"old","domain":"app.example.com","scheme":"http","port":18080,"upstream":"http://127.0.0.1:8080","enabled":true}],"certificates":[],"ddns":[]}' WHERE id=1`)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.Load()
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Routes) != 1 || c.Routes[0].Name != "" || c.Routes[0].Domain != "app.example.com" {
		t.Fatal("legacy route changed")
	}
	c.Routes[0].Name = "旧规则备注"
	if _, e = s.Save(c); e != nil {
		t.Fatal(e)
	}
	c, e = s.Load()
	if e != nil || c.Routes[0].Name != "旧规则备注" {
		t.Fatal("name was not saved")
	}
}
