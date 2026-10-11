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
	_, e = db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS config (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL, body TEXT NOT NULL); CREATE TABLE IF NOT EXISTS ddns_status (id TEXT PRIMARY KEY, body TEXT NOT NULL); INSERT OR IGNORE INTO config VALUES(1,0,'{\"revision\":0,\"routes\":[],\"certificates\":[],\"ddns\":[]}');")
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
func (s *Store) Status(id string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec("INSERT INTO ddns_status VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", id, string(b))
	return e
}
func (s *Store) Statuses() (map[string]json.RawMessage, error) {
	rows, e := s.DB.Query("SELECT id,body FROM ddns_status")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var id, b string
		if e = rows.Scan(&id, &b); e != nil {
			return nil, e
		}
		out[id] = json.RawMessage(b)
	}
	return out, rows.Err()
}
