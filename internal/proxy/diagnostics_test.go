package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTargetProbeIsHeadOnlyAndDoesNotFollowRedirect(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodHead || r.URL.Path != "/" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
		w.Header().Set("Location", "/must-not-follow")
		w.WriteHeader(http.StatusFound)
	}))
	defer s.Close()
	r, err := TestTarget(context.Background(), s.URL)
	if err != nil || !r.Reachable || r.Status != 302 || calls != 1 {
		t.Fatal(r, err, calls)
	}
	for _, invalid := range []string{"file:///tmp/a", s.URL + "/admin", "http://user:secret@localhost", s.URL + "?action=delete"} {
		if _, err := TestTarget(context.Background(), invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestTargetProbeReportsUntrustedTLS(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer s.Close()
	r, err := TestTarget(context.Background(), s.URL)
	if err != nil || r.Reachable || !strings.Contains(r.Message, "证书") {
		t.Fatal(r, err)
	}
}

func TestTargetProbeFailureAndHTTPErrorAreDifferent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	r, err := TestTarget(context.Background(), s.URL)
	if err != nil || !r.Reachable || r.Status != 401 {
		t.Fatal(r, err)
	}
	s.Close()
	r, err = TestTarget(context.Background(), s.URL)
	if err != nil || r.Reachable || r.Message == "" {
		t.Fatal(r, err)
	}
}

func TestLoadErrorsAreActionableAndDoNotExposeRawBody(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"listen tcp :443: bind: address already in use", "端口已被占用"},
		{"listen tcp :80: bind: permission denied", "权限不足"},
		{"sensitive-data", "代理应用失败"},
	} {
		err := loadError(400, tc.body)
		if !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "sensitive-data") {
			t.Fatal(err)
		}
	}
}

func TestStatusReportsOnlyCaddyConfiguredListeners(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apps":{"http":{"servers":{"a":{"listen":[":443"]},"b":{"listen":[":80",":443"]}}}}}`))
	}))
	defer s.Close()
	c := New("missing", t.TempDir(), strings.TrimPrefix(s.URL, "http://"))
	state, err := c.Status(context.Background())
	ports, ok := state["listening_ports"].([]int)
	if err != nil || state["running"] != true || !ok || len(ports) != 2 || ports[0] != 80 || ports[1] != 443 {
		t.Fatal(state, err)
	}
}
