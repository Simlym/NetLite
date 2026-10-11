package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	_ "modernc.org/sqlite"
	"netlite/internal/model"
)

var ErrConflict = errors.New("configuration changed; reload before saving")

type Store struct{ DB *sql.DB }

func Open(path string) (*Store, error) {
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS config (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL, body TEXT NOT NULL); INSERT OR IGNORE INTO config VALUES(1,0,'{\"revision\":0,\"routes\":[],\"certificates\":[]}');")
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Store{db}, nil
}
func (s *Store) Load() (model.Config, error) {
	var c model.Config
	var b string
	e := s.DB.QueryRow("SELECT body FROM config WHERE id=1").Scan(&b)
	if e == nil {
		e = json.Unmarshal([]byte(b), &c)
	}
	return c, e
}
func (s *Store) Save(c model.Config) (model.Config, error) {
	prev := c.Revision
	c.Revision++
	b, e := json.Marshal(c)
	if e != nil {
		return c, e
	}
	r, e := s.DB.Exec("UPDATE config SET revision=?,body=? WHERE id=1 AND revision=?", c.Revision, string(b), prev)
	if e != nil {
		return c, e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return c, ErrConflict
	}
	return c, nil
}
