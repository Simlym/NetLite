package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayPublicPaths(t *testing.T) {
	h := gatewayRoutes(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Path", r.URL.Path)
		w.Header().Set("X-Query", r.URL.RawQuery)
		w.WriteHeader(200)
	}))
	for _, test := range []struct {
		path   string
		status int
		inner  string
	}{
		{"/app/netlite/", 200, "/"},
		{"/app/netlite/api/config?revision=1", 200, "/api/config"},
		{"/app/netlite/assets/app.js", 200, "/assets/app.js"},
		{"/api/config", 404, ""},
		{"/app/netlite-other/api/config", 404, ""},
		{"/app/other", 404, ""},
		{"/app/netlite?panel=confirm", 308, ""},
	} {
		r := httptest.NewRequest("GET", test.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != test.status || w.Header().Get("X-Path") != test.inner {
			t.Fatal(test, w.Code, w.Header())
		}
		if test.status == 308 && w.Header().Get("Location") != "/app/netlite/?panel=confirm" {
			t.Fatal("redirect lost query", w.Header())
		}
	}
}
