package certificates

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Bundle struct{ Certificate, PrivateKey []byte }

func PolicyKey(c model.Certificate) string {
	c.Domains = append([]string(nil), c.Domains...)
	sort.Strings(c.Domains)
	c.RenewBeforeDays = 0
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func privateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return protect(path)
}
func newKey() (crypto.Signer, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}
func parseKey(b []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("invalid private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	s, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("unsupported private key")
	}
	return s, nil
}
func checkBundle(c model.Certificate, b Bundle, now time.Time) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair(b.Certificate, b.PrivateKey)
	if err != nil || len(pair.Certificate) == 0 {
		return nil, fmt.Errorf("certificate and private key do not match")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("invalid certificate")
	}
	if leaf.IsCA || !leaf.NotAfter.After(now) || leaf.NotBefore.After(now.Add(5*time.Minute)) {
		return nil, fmt.Errorf("certificate validity is not usable")
	}
	for _, domain := range c.Domains {
		// Wildcard policies require an actual wildcard SAN, not one covered child.
		found := false
		for _, san := range leaf.DNSNames {
			if san == domain {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("certificate does not contain every requested domain")
		}
	}
	return leaf, nil
}
func (m *Manager) publish(c model.Certificate, b Bundle) (string, *x509.Certificate, error) {
	leaf, err := checkBundle(c, b, m.now())
	if err != nil {
		return "", nil, err
	}
	parent := filepath.Join(m.dir, "material", PolicyKey(c))
	if err = privateDir(parent); err != nil {
		return "", nil, err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return "", nil, err
	}
	generation := hex.EncodeToString(nonce[:])
	path := filepath.Join(parent, generation)
	if err = privateDir(path); err != nil {
		return "", nil, err
	}
	if err = atomicWrite(filepath.Join(path, "cert.pem"), b.Certificate); err != nil {
		return "", nil, err
	}
	if err = atomicWrite(filepath.Join(path, "key.pem"), b.PrivateKey); err != nil {
		return "", nil, err
	}
	for _, dir := range []string{parent, filepath.Dir(parent), m.dir} {
		if err = syncDirectory(dir); err != nil {
			return "", nil, err
		}
	}
	return generation, leaf, nil
}
func (m *Manager) Files(c model.Certificate) (model.CertificateFiles, error) {
	r, err := m.load(c)
	if err != nil {
		return model.CertificateFiles{}, err
	}
	if len(r.Generation) != 32 {
		return model.CertificateFiles{}, fmt.Errorf("证书 %s 尚未签发，请先申请证书", c.ID)
	}
	if _, err = hex.DecodeString(r.Generation); err != nil {
		return model.CertificateFiles{}, fmt.Errorf("invalid certificate generation")
	}
	dir := filepath.Join(m.dir, "material", PolicyKey(c), r.Generation)
	files := model.CertificateFiles{CertificateFile: filepath.Join(dir, "cert.pem"), PrivateKeyFile: filepath.Join(dir, "key.pem")}
	if _, err = os.Stat(files.CertificateFile); err != nil {
		return model.CertificateFiles{}, fmt.Errorf("certificate file missing")
	}
	if _, err = os.Stat(files.PrivateKeyFile); err != nil {
		return model.CertificateFiles{}, fmt.Errorf("private key file missing")
	}
	return files, nil
}
func (m *Manager) RequireReady(cfg model.Config) error {
	certs := map[string]model.Certificate{}
	for _, c := range cfg.Certificates {
		certs[c.ID] = c
	}
	for _, route := range cfg.Routes {
		if !route.Enabled || route.Scheme != "https" {
			continue
		}
		c := certs[route.CertificateID]
		r, err := m.load(c)
		if err != nil {
			return err
		}
		if !r.State.NotAfter.After(m.now()) {
			return fmt.Errorf("证书 %s 尚未就绪或已过期", c.ID)
		}
		if _, err = m.Files(c); err != nil {
			return err
		}
	}
	return nil
}
