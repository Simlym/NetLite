package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func gatewayRequest(method string) *http.Request {
	r := httptest.NewRequest(method, "http://nas.local/app/netlite/api/config", nil)
	r = r.WithContext(context.WithValue(r.Context(), gatewayConnectionKey{}, true))
	r.Header.Set("X-Trim-Userid", "1000")
	r.Header.Set("X-Trim-Username", "管理员")
	r.Header.Set("X-Trim-Isadmin", "true")
	return r
}

func TestGatewayIdentityAndRequestProtection(t *testing.T) {
	g, err := NewGateway()
	if err != nil {
		t.Fatal(err)
	}
	p, err := g.Authenticate(gatewayRequest("GET"))
	if err != nil || p.UID != "1000" || p.Name != "管理员" || !p.IsAdmin || p.Source != "fnos-gateway" || p.CSRF == "" {
		t.Fatal("invalid platform identity", p, err)
	}
	tests := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"TCP spoof", func(r *http.Request) { *r = *r.WithContext(context.Background()) }},
		{"missing UID", func(r *http.Request) { r.Header.Del("X-Trim-Userid") }},
		{"invalid UID", func(r *http.Request) { r.Header.Set("X-Trim-Userid", "-1") }},
		{"noncanonical UID", func(r *http.Request) { r.Header.Set("X-Trim-Userid", "01000") }},
		{"missing admin", func(r *http.Request) { r.Header.Del("X-Trim-Isadmin") }},
		{"invalid admin", func(r *http.Request) { r.Header.Set("X-Trim-Isadmin", "TRUE") }},
		{"duplicate identity", func(r *http.Request) { r.Header.Add("X-Trim-Userid", "1001") }},
		{"invalid username", func(r *http.Request) { r.Header.Set("X-Trim-Username", "admin\nother") }},
		{"cross origin", func(r *http.Request) { r.Header.Set("Origin", "https://attacker.example") }},
		{"cross site", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := gatewayRequest("GET")
			test.mutate(r)
			if _, err := g.Authenticate(r); err == nil {
				t.Fatal("untrusted identity accepted")
			}
		})
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			r := gatewayRequest(method)
			if _, err := g.Authenticate(r); err == nil {
				t.Fatal("write accepted without CSRF")
			}
			r.Header.Set("X-CSRF-Token", p.CSRF)
			r.Header.Set("Origin", "https://nas.local")
			if _, err := g.Authenticate(r); err != nil {
				t.Fatal(err)
			}
			r.Header.Set("X-Trim-Userid", "1001")
			if _, err := g.Authenticate(r); err == nil {
				t.Fatal("another user's CSRF accepted")
			}
		})
	}
	other, _ := NewGateway()
	r := gatewayRequest("POST")
	r.Header.Set("X-CSRF-Token", p.CSRF)
	if _, err := other.Authenticate(r); err == nil {
		t.Fatal("restart kept old CSRF token")
	}
}

func TestGatewayAdminBoundaryAndNoLocalLogin(t *testing.T) {
	g, _ := NewGateway()
	handler := g.RequireAdmin(http.HandlerFunc(g.Handler))
	for _, test := range []struct {
		path, admin string
		status      int
	}{
		{"/api/auth/state", "true", 200},
		{"/api/auth/me", "true", 200},
		{"/api/auth/me", "false", 403},
		{"/api/auth/login", "true", 404},
		{"/api/auth/setup", "true", 404},
		{"/api/auth/logout", "true", 404},
	} {
		r := gatewayRequest("GET")
		r.URL.Path = test.path
		r.Header.Set("X-Trim-Isadmin", test.admin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != test.status || len(w.Result().Cookies()) != 0 {
			t.Fatal(test, w.Code, w.Body.String())
		}
	}
	called := false
	r := gatewayRequest("GET")
	r.Header.Set("X-Trim-Isadmin", "false")
	w := httptest.NewRecorder()
	g.RequireAdmin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })).ServeHTTP(w, r)
	if called || w.Code != 403 {
		t.Fatal("ordinary NAS user reached app")
	}
}
