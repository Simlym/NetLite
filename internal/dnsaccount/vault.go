// Package dnsaccount owns shared encrypted DNS credentials for DDNS and certificates.
package dnsaccount

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"netlite/internal/model"
	"netlite/internal/store"
	"os"
	"path/filepath"
)

type Vault struct {
	store *store.Store
	key   []byte
}

// Credentials share one application-owned vault for DDNS and DNS-01.
func Open(s *store.Store, dir string) (*Vault, error) {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS dns_accounts(id TEXT PRIMARY KEY,body TEXT NOT NULL,secret BLOB NOT NULL);`); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := protect(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "master.key")
	key, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		var count int
		if err = s.DB.QueryRow("SELECT COUNT(*) FROM dns_accounts").Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, fmt.Errorf("DNS master key missing; restore it from backup")
		}
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, err
		}
		var f *os.File
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid DNS master key")
	}
	return &Vault{store: s, key: key}, nil
}
func (s *Vault) crypt() cipher.AEAD {
	b, _ := aes.NewCipher(s.key)
	a, _ := cipher.NewGCM(b)
	return a
}
func (s *Vault) Encrypt(id, secret string) ([]byte, error) {
	a := s.crypt()
	nonce := make([]byte, a.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return nil, e
	}
	return a.Seal(nonce, nonce, []byte(secret), []byte(id)), nil
}
func (s *Vault) Decrypt(id string, data []byte) (string, error) {
	a := s.crypt()
	n := a.NonceSize()
	if len(data) < n {
		return "", fmt.Errorf("invalid encrypted credential")
	}
	b, e := a.Open(nil, data[:n], data[n:], []byte(id))
	if e != nil {
		return "", fmt.Errorf("cannot decrypt credential")
	}
	return string(b), nil
}
func (s *Vault) List() ([]model.DNSAccount, error) {
	rows, e := s.store.DB.Query("SELECT body FROM dns_accounts ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.DNSAccount{}
	for rows.Next() {
		var b string
		var a model.DNSAccount
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &a); e != nil {
			return nil, e
		}
		a.HasSecret = true
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Vault) Resolve(id string) (model.DNSCredential, error) {
	var body string
	var sealed []byte
	if e := s.store.DB.QueryRow("SELECT body,secret FROM dns_accounts WHERE id=?", id).Scan(&body, &sealed); e != nil {
		return model.DNSCredential{}, fmt.Errorf("DNS 账号不存在")
	}
	var a model.DNSAccount
	if e := json.Unmarshal([]byte(body), &a); e != nil {
		return model.DNSCredential{}, fmt.Errorf("DNS 账号数据损坏")
	}
	secret, e := s.Decrypt(id, sealed)
	if e != nil {
		return model.DNSCredential{}, e
	}
	return model.DNSCredential{Provider: a.Provider, ID: a.CredentialID, Secret: secret}, nil
}

