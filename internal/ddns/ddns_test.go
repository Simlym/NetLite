package ddns

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"netlite/internal/model"
	"strings"
	"testing"
)

func TestCloudflarePreservesRecordAttributes(t *testing.T) {
	patches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing credential")
		}
		if r.Method == "GET" {
			io.WriteString(w, "{\"success\":true,\"result\":{\"name\":\"app.example.com\",\"type\":\"A\",\"content\":\"192.0.2.1\"}}")
			return
		}
		if r.Method != "PATCH" {
			t.Error("expected PATCH")
		}
		b, _ := io.ReadAll(r.Body)
		if string(b) != "{\"content\":\"192.0.2.2\"}" {
			t.Error("must patch content only")
		}
		patches++
		io.WriteString(w, "{\"success\":true,\"result\":{}}")
	}))
	defer server.Close()
	t.Setenv("TEST_CF_TOKEN", "test-secret")
	p := Cloudflare{server.Client(), server.URL}
	d := model.DDNS{Name: "app.example.com", Type: "A", ZoneID: "zone", RecordID: "record", TokenEnv: "TEST_CF_TOKEN"}
	if e := p.Update(context.Background(), d, "192.0.2.2"); e != nil {
		t.Fatal(e)
	}
	if patches != 1 {
		t.Fatal("not updated")
	}
	d.Name = "wrong.example.com"
	if e := p.Update(context.Background(), d, "192.0.2.2"); e == nil || !strings.Contains(e.Error(), "does not match") {
		t.Fatal("mismatched record accepted")
	}
	if patches != 1 {
		t.Fatal("modified mismatched record")
	}
}
