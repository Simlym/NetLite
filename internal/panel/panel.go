// Package panel provides management TLS independently of the business proxy.
package panel

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"netlite/internal/certificates"
	"netlite/internal/model"
	"netlite/internal/store"
)

type Settings struct {
	Enabled bool `json:"enabled"`
	Domain string `json:"domain"`
	CertificateID string `json:"certificate_id"`
	Redirect bool `json:"redirect"`
	Policy model.Certificate `json:"policy,omitempty"`
}
type Manager struct {
	DB *store.Store
	Certificates *certificates.Manager
	Port int
	mu sync.Mutex
	settings Settings
	pending *Settings
	timer *time.Timer
 previous *tls.Certificate
	cert atomic.Pointer[tls.Certificate]
}
func Open(db *store.Store, certs *certificates.Manager, port int) (*Manager,error) {
	m := &Manager{DB:db,Certificates:certs,Port:port}
	if _,err := db.DB.Exec("CREATE TABLE IF NOT EXISTS panel_settings(id INTEGER PRIMARY KEY CHECK(id=1),body TEXT NOT NULL)"); err != nil { return nil,err }
	var body string
	err := db.DB.QueryRow("SELECT body FROM panel_settings WHERE id=1").Scan(&body)
	if err != nil && err != sql.ErrNoRows { return nil,err }
	if err == nil { if err = json.Unmarshal([]byte(body),&m.settings); err != nil { return nil,err } }
	if m.settings.Enabled { cert,err := m.load(m.settings); if err != nil { return nil,fmt.Errorf("面板证书加载失败，请执行 netlite reset-https: %w",err) }; m.cert.Store(cert) }
	return m,nil
}
func (m *Manager) load(s Settings) (*tls.Certificate,error) {
	files,err := m.Certificates.Files(s.Policy); if err != nil { return nil,err }
	cert,err := tls.LoadX509KeyPair(files.CertificateFile,files.PrivateKeyFile); if err != nil { return nil,err }
	leaf,err := x509.ParseCertificate(cert.Certificate[0]); if err != nil { return nil,err }
	if err = leaf.VerifyHostname(s.Domain); err != nil { return nil,fmt.Errorf("证书不覆盖访问域名 %s",s.Domain) }
	if now := time.Now(); now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) { return nil,fmt.Errorf("证书尚未生效或已过期") }
	cert.Leaf = leaf; return &cert,nil
}
func (m *Manager) Reload(context.Context) error {
	m.mu.Lock(); defer m.mu.Unlock()
	s := m.settings; if m.pending != nil { s = *m.pending }
	if !s.Enabled { return nil }
	cert,err := m.load(s); if err != nil { return err }; m.cert.Store(cert); return nil
}
func (m *Manager) ValidateConfig(cfg model.Config) error {
	m.mu.Lock(); defer m.mu.Unlock()
	for _,s := range []Settings{m.settings, func() Settings { if m.pending != nil { return *m.pending }; return Settings{} }()} {
		if !s.Enabled { continue }
		found := false
		for _,c := range cfg.Certificates { if c.ID == s.CertificateID && certificates.PolicyKey(c) == certificates.PolicyKey(s.Policy) { found = true } }
		if !found { return fmt.Errorf("证书正在供管理面板使用，请先更换或停用面板 HTTPS") }
	}
	return nil
}
func (m *Manager) persist(s Settings) error {
	body,err := json.Marshal(s); if err != nil { return err }
	_,err = m.DB.DB.Exec("INSERT INTO panel_settings VALUES(1,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body",string(body))
	if err == nil { m.settings = s }; return err
}
func (m *Manager) rollback() {
	m.pending = nil
	if !m.settings.Enabled { m.cert.Store(nil); return }
	m.cert.Store(m.previous)
}
func (m *Manager) Close() { m.mu.Lock(); defer m.mu.Unlock(); if m.timer != nil { m.timer.Stop() }; m.rollback() }
func (m *Manager) Address(s Settings) string { return "https://"+net.JoinHostPort(s.Domain,strconv.Itoa(m.Port)) }
func output(w http.ResponseWriter, status int, value any) { w.Header().Set("Content-Type","application/json"); w.Header().Set("Cache-Control","no-store"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(value) }
func (m *Manager) Handler(w http.ResponseWriter,r *http.Request) {
	m.mu.Lock(); defer m.mu.Unlock()
	fail := func(err error) { output(w,422,map[string]string{"error":err.Error()}) }
	switch {
	case r.URL.Path == "/api/panel" && r.Method == "GET":
		s := m.settings; if m.pending != nil { s = *m.pending }
		output(w,200,map[string]any{"enabled":s.Enabled,"domain":s.Domain,"certificate_id":s.CertificateID,"redirect":s.Redirect,"port":m.Port,"pending":m.pending != nil,"url":m.Address(s),"rollback_seconds":120})
	case r.URL.Path == "/api/panel" && r.Method == "PUT":
		if m.pending != nil { fail(fmt.Errorf("请先确认或取消正在测试的 HTTPS 设置")); return }
		var req struct { Enabled bool `json:"enabled"`; Domain string `json:"domain"`; CertificateID string `json:"certificate_id"`; Redirect bool `json:"redirect"` }
		dec := json.NewDecoder(http.MaxBytesReader(w,r.Body,8192)); dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil { fail(err); return }
		if !req.Enabled {
			if err := m.persist(Settings{}); err != nil { fail(err); return }; m.cert.Store(nil); output(w,200,map[string]any{"pending":false,"url":"http://"+r.Host}); return
		}
		domain := strings.ToLower(strings.TrimSpace(req.Domain))
		if !model.Domain(domain) || strings.HasPrefix(domain,"*.") { fail(fmt.Errorf("请输入具体访问域名，不含协议或端口")); return }
		cfg,err := m.DB.Load(); if err != nil { fail(err); return }
		s := Settings{Enabled:true,Domain:domain,CertificateID:req.CertificateID,Redirect:req.Redirect}
		for _,c := range cfg.Certificates { if c.ID == req.CertificateID { s.Policy = c } }
		if s.Policy.ID == "" { fail(fmt.Errorf("请选择已签发的证书")); return }
		cert,err := m.load(s); if err != nil { fail(err); return }
		m.previous = m.cert.Load(); m.pending = &s; m.cert.Store(cert)
		m.timer = time.AfterFunc(120*time.Second,func(){ m.mu.Lock(); defer m.mu.Unlock(); m.rollback() })
		output(w,200,map[string]any{"pending":true,"url":m.Address(s),"rollback_seconds":120})
	case r.URL.Path == "/api/panel/confirm" && r.Method == "POST":
		if m.pending == nil { fail(fmt.Errorf("没有等待确认的设置")); return }
		if r.TLS == nil || r.Host != net.JoinHostPort(m.pending.Domain,strconv.Itoa(m.Port)) { fail(fmt.Errorf("请通过新的 HTTPS 地址登录后确认")); return }
		if err := m.persist(*m.pending); err != nil { fail(err); return }; m.timer.Stop(); m.pending = nil; output(w,200,map[string]bool{"ok":true})
	case r.URL.Path == "/api/panel/cancel" && r.Method == "POST":
		if m.timer != nil { m.timer.Stop() }; m.rollback(); output(w,200,map[string]bool{"ok":true})
	default: http.NotFound(w,r)
	}
}
func (m *Manager) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		m.mu.Lock(); s,pending := m.settings,m.pending != nil; m.mu.Unlock()
		if s.Enabled && !pending && r.TLS == nil {
			// HTTP never receives credentials after HTTPS has been confirmed.
			if !s.Redirect || (r.Method != "GET" && r.Method != "HEAD") { output(w,400,map[string]string{"error":"请通过 HTTPS 访问管理接口"}); return }
			target := m.Address(s)+r.URL.RequestURI(); if _,err := url.Parse(target); err != nil { http.Error(w,"invalid address",400); return }
			http.Redirect(w,r,target,http.StatusTemporaryRedirect); return
		}
		if r.TLS != nil { w.Header().Set("Cache-Control","no-store") }
		next.ServeHTTP(w,r)
	})
}
// Listener accepts HTTP and TLS on the same management port. Protocol sniffing
// has a bounded concurrency and deadline; TLS certificates can be swapped live.
func (m *Manager) Listener(base net.Listener) net.Listener {
	l := &mixedListener{Listener:base,manager:m,ready:make(chan net.Conn),done:make(chan struct{}),slots:make(chan struct{},64)}
	go l.run(); return l
}
type mixedListener struct {
	net.Listener
	manager *Manager
	ready chan net.Conn
	done chan struct{}
	once sync.Once
	slots chan struct{}
}
type bufferedConn struct { net.Conn; reader *bufio.Reader }
func (c *bufferedConn) Read(b []byte) (int,error) { return c.reader.Read(b) }
func (l *mixedListener) run() {
	defer l.Close()
	for {
		c,err := l.Listener.Accept(); if err != nil { return }
		select { case l.slots<-struct{}{}: case <-l.done: c.Close(); return; default: c.Close(); continue }
		go func(){
			defer func(){ <-l.slots }()
			_ = c.SetReadDeadline(time.Now().Add(5*time.Second)); reader := bufio.NewReader(c)
			b,err := reader.Peek(1); if err != nil { c.Close(); return }; _ = c.SetReadDeadline(time.Time{})
			var conn net.Conn = &bufferedConn{Conn:c,reader:reader}
			if b[0] == 22 {
				if l.manager.cert.Load() == nil { c.Close(); return }
				conn = tls.Server(conn,&tls.Config{MinVersion:tls.VersionTLS12,GetCertificate:func(*tls.ClientHelloInfo)(*tls.Certificate,error){ cert := l.manager.cert.Load(); if cert == nil { return nil,fmt.Errorf("HTTPS disabled") }; return cert,nil }})
			}
			select { case l.ready<-conn: case <-l.done: c.Close() }
		}()
	}
}
func (l *mixedListener) Accept() (net.Conn,error) { select { case c := <-l.ready: return c,nil; case <-l.done: return nil,net.ErrClosed } }
func (l *mixedListener) Close() error { var err error; l.once.Do(func(){close(l.done);err=l.Listener.Close()}); return err }
