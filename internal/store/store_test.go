package store

import (
	"errors"
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
	if e != nil || c.Revision != 1 {
		t.Fatal("not persisted")
	}
}
