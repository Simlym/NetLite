package auth

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"netlite/internal/store"
	"path/filepath"
	"testing"
)

func testAuth(t *testing.T) *Manager {
	t.Helper()
	s,err := store.Open(filepath.Join(t.TempDir(),"test.db")); if err != nil { t.Fatal(err) }; t.Cleanup(func(){s.DB.Close()})
	m,err := Open(s.DB,"bootstrap-secret"); if err != nil { t.Fatal(err) }; return m
}
func request(m *Manager,path,method string,body any,cookie *http.Cookie,csrf string,secure bool) *httptest.ResponseRecorder {
	data,_ := json.Marshal(body)
	r := httptest.NewRequest(method,"http://localhost/api/auth/"+path,bytes.NewReader(data))
	r.Header.Set("Content-Type","application/json"); r.Header.Set("X-CSRF-Token",csrf)
	if cookie != nil { r.AddCookie(cookie) }; if secure { r.TLS = &tls.ConnectionState{} }
	w := httptest.NewRecorder();m.Handler(w,r);return w
}
func TestBootstrapSessionCSRFAndPasswordReset(t *testing.T) {
	m := testAuth(t)
	bad := request(m,"setup","POST",map[string]any{"username":"admin","password":"correct horse battery","code":"wrong"},nil,"",false)
	if bad.Code != 401 { t.Fatal(bad.Code) }
	w := request(m,"setup","POST",map[string]any{"username":"admin","password":"correct horse battery","code":"bootstrap-secret","remember":true},nil,"",true)
	if w.Code != 200 { t.Fatal(w.Body.String()) }
	cookies := w.Result().Cookies(); if len(cookies)!=1 { t.Fatal("missing session cookie") }; cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge<=0 { t.Fatal("unsafe cookie",cookie) }
	if w = request(m,"setup","POST",map[string]any{"username":"other","password":"another password","code":"bootstrap-secret"},nil,"",false); w.Code != 409 { t.Fatal("bootstrap reusable") }
	w = request(m,"me","GET",nil,cookie,"",true)
	var p Principal; if err := json.Unmarshal(w.Body.Bytes(),&p); err != nil || p.Name!="admin" || p.CSRF=="" { t.Fatal(w.Body.String()) }
	if w = request(m,"logout","POST",nil,cookie,"",true); w.Code != 401 { t.Fatal("missing CSRF accepted") }
	if w = request(m,"password","POST",map[string]string{"old_password":"wrong","new_password":"different long password"},cookie,p.CSRF,true); w.Code != 401 { t.Fatal("wrong old password accepted") }
	if err := m.Reset("new correct password",true); err != nil { t.Fatal(err) }
	if w = request(m,"me","GET",nil,cookie,"",true); w.Code != 401 { t.Fatal("reset kept old session") }
	w = request(m,"login","POST",map[string]string{"username":"admin","password":"new correct password"},nil,"",true)
	if w.Code!=200 { t.Fatal(w.Body.String()) }; cookie=w.Result().Cookies()[0]
	w=request(m,"me","GET",nil,cookie,"",true); _=json.Unmarshal(w.Body.Bytes(),&p)
	if !p.MustChange { t.Fatal("temporary password bypassed") }
	w=request(m,"password","POST",map[string]string{"old_password":"new correct password","new_password":"personal long password"},cookie,p.CSRF,true)
	if w.Code!=200 { t.Fatal(w.Body.String()) }; cookie=w.Result().Cookies()[0]
	w=request(m,"me","GET",nil,cookie,"",true); _=json.Unmarshal(w.Body.Bytes(),&p)
	if p.MustChange { t.Fatal("password change did not complete") }
	if w=request(m,"logout","POST",nil,cookie,p.CSRF,true); w.Code!=200 { t.Fatal(w.Body.String()) }
	if w=request(m,"me","GET",nil,cookie,"",true); w.Code!=401 { t.Fatal("logout kept session") }
}
func TestCrossSiteRateLimitAndExpiry(t *testing.T) {
	m:=testAuth(t); if err:=m.Create("admin","long enough password",false); err!=nil { t.Fatal(err) }
	r:=httptest.NewRequest("POST","http://localhost/api/auth/login",bytes.NewBufferString(`{"username":"admin","password":"long enough password"}`))
	r.Header.Set("Origin","https://attacker.example");r.Header.Set("Content-Type","application/json")
	w:=httptest.NewRecorder();m.Handler(w,r);if w.Code!=403 { t.Fatal("cross-site login accepted") }
	w=request(m,"login","POST",map[string]string{"username":"admin","password":"long enough password"},nil,"",false)
	cookie:=w.Result().Cookies()[0]
	if _,err:=m.DB.Exec("UPDATE auth_session SET idle_until=0");err!=nil { t.Fatal(err) }
	if w=request(m,"me","GET",nil,cookie,"",false);w.Code!=401 { t.Fatal("expired session accepted") }
	for i:=0;i<10;i++ { w=request(m,"login","POST",map[string]string{"username":"admin","password":"wrong"},nil,"",false) }
	if w.Code!=429 { t.Fatal("login attempts not limited",w.Code) }
}
