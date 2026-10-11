package ddnsgo

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"netlite/internal/model"
	"netlite/internal/store"
	"os"
	"path/filepath"
	"testing"
)

type fakeEngine struct {
	running map[string]bool
	starts  int
}

func (e *fakeEngine) Start(_ context.Context, t Task) error {
	e.running[t.ID] = true
	e.starts++
	return nil
}
func (e *fakeEngine) Stop(id string)        { delete(e.running, id) }
func (e *fakeEngine) Close()                { clear(e.running) }
func (e *fakeEngine) State(id string) State { return State{Running: e.running[id]} }
func request(t *testing.T, h http.Handler, method, path string, body any, want int) []byte {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}
func TestServiceLifecycleAndCredentials(t *testing.T) {
	dir := t.TempDir()
	db, e := store.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.DB.Close()
	binary := filepath.Join(dir, "fake.exe")
	os.WriteFile(binary, []byte("fixture"), 0600)
	svc, e := OpenService(context.Background(), db, binary, filepath.Join(dir, "private"))
	if e != nil {
		t.Fatal(e)
	}
	engine := &fakeEngine{running: map[string]bool{}}
	svc.engine = engine
	h := svc.Router()
	account := accountInput{DNSAccount: model.DNSAccount{ID: "account", Name: "DNSPod", Provider: "dnspod", CredentialID: "12345"}, Secret: "dummy-secret"}
	response := request(t, h, "PUT", "/accounts/account", account, 200)
	if bytes.Contains(response, []byte("dummy-secret")) {
		t.Fatal("secret returned")
	}
	var sealed []byte
	if e = db.DB.QueryRow("SELECT secret FROM dns_accounts WHERE id='account'").Scan(&sealed); e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(sealed, []byte("dummy-secret")) {
		t.Fatal("secret stored in plaintext")
	}
	account.Secret = ""
	request(t, h, "PUT", "/accounts/account", account, 200)
	task := model.DDNSTask{ID: "task", Name: "家庭网络", AccountID: "account", IPv4Domains: []string{"example.com", "*.example.com"}, IPv4URL: "https://api4.ipify.org", Interval: 300, TTL: "600"}
	request(t, h, "PUT", "/tasks/task", task, 200)
	if engine.starts != 0 {
		t.Fatal("disabled task started")
	}
	task.Enabled = true
	request(t, h, "PUT", "/tasks/task", task, 200)
	if !engine.running["task"] {
		t.Fatal("enabled task not started")
	}
	request(t, h, "DELETE", "/accounts/account", nil, 409)
	duplicate := task
	duplicate.ID = "duplicate"
	request(t, h, "PUT", "/tasks/duplicate", duplicate, 400)
	legacy := model.Config{DDNS: []model.DDNS{{Name: "example.com", Type: "A", Enabled: true}}}
	if svc.ValidateLegacy(legacy) == nil {
		t.Fatal("legacy duplicate accepted")
	}
	request(t, h, "POST", "/tasks/task/run", nil, 202)
	if engine.starts != 2 {
		t.Fatal("manual request did not reload")
	}
	task.Enabled = false
	request(t, h, "PUT", "/tasks/task", task, 200)
	if engine.running["task"] {
		t.Fatal("disabled child running")
	}
	reopened, e := OpenService(context.Background(), db, binary, filepath.Join(dir, "private"))
	if e != nil {
		t.Fatal(e)
	}
	loaded, e := reopened.renderTask(task)
	if e != nil || loaded.Secret != "dummy-secret" {
		t.Fatal("credential did not survive reload")
	}
	request(t, h, "DELETE", "/tasks/task", nil, 200)
	request(t, h, "DELETE", "/accounts/account", nil, 200)
}
func TestMissingMasterKeyDoesNotReplaceCredentials(t *testing.T) {
	dir := t.TempDir()
	db, e := store.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.DB.Close()
	private := filepath.Join(dir, "private")
	s, e := OpenService(context.Background(), db, "", private)
	if e != nil {
		t.Fatal(e)
	}
	request(t, s.Router(), "PUT", "/accounts/account", accountInput{DNSAccount: model.DNSAccount{Name: "CF", Provider: "cloudflare"}, Secret: "dummy"}, 200)
	if e = os.Remove(filepath.Join(private, "master.key")); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenService(context.Background(), db, "", private); e == nil {
		t.Fatal("lost master key silently replaced")
	}
}
