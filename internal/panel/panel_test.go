package panel

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"netlite/internal/certificates"
	"netlite/internal/model"
	"netlite/internal/store"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func testPanel(t *testing.T) (*Manager, model.Config) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	certs, err := certificates.Open(ctx, db, filepath.Join(t.TempDir(), "certs"), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := model.Certificate{ID: "local", Domains: []string{"localhost"}, Mode: "internal"}
	cfg, err := db.Save(model.Config{Certificates: []model.Certificate{c}})
	if err != nil {
		t.Fatal(err)
	}
	go func() { defer close(done); certs.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	if err = certs.Request(c, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = certs.Files(c); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	m, err := Open(db, certs, 18780)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, cfg
}
func panelRequest(m *Manager, path, method string, body any, secure bool) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(method, "http://localhost:"+strconv.Itoa(m.Port)+path, bytes.NewReader(data))
	if secure {
		r.TLS = &tls.ConnectionState{}
	}
	w := httptest.NewRecorder()
	m.Handler(w, r)
	return w
}
func TestPanelValidationConfirmationAndRestart(t *testing.T) {
	m, cfg := testPanel(t)
	req := map[string]any{"enabled": true, "domain": "wrong.example", "certificate_id": "local", "redirect": true}
	if w := panelRequest(m, "/api/panel", "PUT", req, false); w.Code != 422 {
		t.Fatal("mismatched hostname accepted")
	}
	req["domain"] = "localhost"
	if w := panelRequest(m, "/api/panel", "PUT", req, false); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := panelRequest(m, "/api/panel/confirm", "POST", nil, false); w.Code != 422 {
		t.Fatal("confirmation through HTTP accepted")
	}
	if err := m.ValidateConfig(model.Config{}); err == nil {
		t.Fatal("pending certificate deletion accepted")
	}
	if w := panelRequest(m, "/api/panel/confirm", "POST", nil, true); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := m.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.ValidateConfig(model.Config{}); err == nil {
		t.Fatal("active certificate deletion accepted")
	}
	second, err := Open(m.DB, m.Certificates, m.Port)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.cert.Load() == nil || !second.settings.Enabled {
		t.Fatal("restart lost TLS")
	}
	w := httptest.NewRecorder()
	second.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, httptest.NewRequest("GET", "http://attacker.example/api/config", nil))
	if w.Code != 307 || w.Header().Get("Location") != "https://localhost:18780/api/config" {
		t.Fatal("unsafe HTTP redirect", w.Code, w.Header())
	}
}
func TestSharedPortTLSAndRollback(t *testing.T) {
	m, _ := testPanel(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	m.Port = ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: m.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "panel") })), ReadHeaderTimeout: time.Second}
	go srv.Serve(m.Listener(ln))
	defer srv.Close()
	url := "http://localhost:" + strconv.Itoa(m.Port)
	w := panelRequest(m, "/api/panel", "PUT", map[string]any{"enabled": true, "domain": "localhost", "certificate_id": "local", "redirect": true}, false)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	root, err := m.Certificates.RootCertificate()
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(root)
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	defer client.CloseIdleConnections()
	res, err := client.Get("https" + url[4:])
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.TLS == nil {
		t.Fatal("not TLS")
	}
	res, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.TLS != nil {
		t.Fatal("HTTP disabled before confirmation")
	}
	if w = panelRequest(m, "/api/panel/cancel", "POST", nil, false); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if m.cert.Load() != nil || m.pending != nil {
		t.Fatal("rollback did not restore HTTP")
	}
	res, err = client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
}
