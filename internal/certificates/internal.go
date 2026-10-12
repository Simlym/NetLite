package certificates

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"time"
)

type rootRecord struct{ Certificate, PrivateKey []byte }

func (m *Manager) root() (rootRecord, error) {
	path := filepath.Join(m.dir, "internal-ca.json")
	b, err := os.ReadFile(path)
	if err == nil {
		var r rootRecord
		err = json.Unmarshal(b, &r)
		return r, err
	}
	if !os.IsNotExist(err) {
		return rootRecord{}, err
	}
	key, keyPEM, err := newKey()
	if err != nil {
		return rootRecord{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return rootRecord{}, err
	}
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "NetLite Internal CA"}, NotBefore: m.now().Add(-5 * time.Minute), NotAfter: m.now().AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, MaxPathLen: 0, MaxPathLenZero: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return rootRecord{}, err
	}
	r := rootRecord{Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), PrivateKey: keyPEM}
	b, err = json.Marshal(r)
	if err == nil {
		err = atomicWrite(path, b)
	}
	return r, err
}
func (m *Manager) issueInternal(ctx context.Context, c model.Certificate) (Bundle, error) {
	if err := ctx.Err(); err != nil {
		return Bundle{}, err
	}
	m.rootMu.Lock()
	r, err := m.root()
	m.rootMu.Unlock()
	if err != nil {
		return Bundle{}, err
	}
	block, _ := pem.Decode(r.Certificate)
	if block == nil {
		return Bundle{}, fmt.Errorf("invalid internal CA")
	}
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Bundle{}, err
	}
	if root.NotAfter.Before(m.now().Add(90 * 24 * time.Hour)) {
		return Bundle{}, fmt.Errorf("internal CA needs replacement")
	}
	rootKey, err := parseKey(r.PrivateKey)
	if err != nil {
		return Bundle{}, err
	}
	key, keyPEM, err := newKey()
	if err != nil {
		return Bundle{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Bundle{}, err
	}
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: c.Domains[0]}, DNSNames: c.Domains, NotBefore: m.now().Add(-5 * time.Minute), NotAfter: m.now().Add(90 * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, root, key.Public(), rootKey)
	if err != nil {
		return Bundle{}, err
	}
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), r.Certificate...)
	return Bundle{Certificate: chain, PrivateKey: keyPEM}, nil
}
func (m *Manager) RootCertificate() ([]byte, error) {
	m.rootMu.Lock()
	defer m.rootMu.Unlock()
	r, err := m.root()
	return r.Certificate, err
}
