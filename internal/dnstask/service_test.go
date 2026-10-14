package dnstask

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
	"strings"
	"testing"
)

type fakeEngine struct {
	running map[string]bool
	starts  int
	checks  int
}

func (e *fakeEngine) Start(_ context.Context, t Task) error {
	e.running[t.ID] = true
	e.starts++
	return nil
}
func (e *fakeEngine) CheckNow(id string) error { e.checks++; return nil }
func (e *fakeEngine) Stop(id string)           { delete(e.running, id) }
func (e *fakeEngine) Close()                   { clear(e.running) }
func (e *fakeEngine) State(id string) State    { return State{Running: e.running[id]} }
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
	svc, e := OpenService(context.Background(), db, filepath.Join(dir, "private"))
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
	request(t, h, "POST", "/tasks/task/run", nil, 202)
	if engine.starts != 1 || engine.checks != 1 {
		t.Fatal("manual request restarted the task or did not queue a check")
	}
	task.Enabled = false
	request(t, h, "PUT", "/tasks/task", task, 200)
	if engine.running["task"] {
		t.Fatal("disabled task running")
	}
	reopened, e := OpenService(context.Background(), db, filepath.Join(dir, "private"))
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
	s, e := OpenService(context.Background(), db, private)
	if e != nil {
		t.Fatal(e)
	}
	request(t, s.Router(), "PUT", "/accounts/account", accountInput{DNSAccount: model.DNSAccount{Name: "CF", Provider: "cloudflare"}, Secret: "dummy"}, 200)
	if e = os.Remove(filepath.Join(private, "master.key")); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenService(context.Background(), db, private); e == nil {
		t.Fatal("lost master key silently replaced")
	}
}

func TestTaskNameIsOptional(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	svc, err := OpenService(context.Background(), db, filepath.Join(dir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	svc.engine = &fakeEngine{running: map[string]bool{}}
	h := svc.Router()
	request(t, h, "PUT", "/accounts/account", accountInput{DNSAccount: model.DNSAccount{Name: "CF", Provider: "cloudflare"}, Secret: "dummy"}, 200)
	task := model.DDNSTask{ID: "task", AccountID: "account", IPv4Domains: []string{"home.example.com"}, IPv4URL: "https://api4.ipify.org", Interval: 300, TTL: "600"}
	for _, name := range []string{"", "  ", "  家庭网络  "} {
		task.Name = name
		var saved model.DDNSTask
		if err := json.Unmarshal(request(t, h, "PUT", "/tasks/task", task, 200), &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Name != strings.TrimSpace(name) {
			t.Fatalf("name = %q", saved.Name)
		}
		loaded, err := svc.tasks()
		if err != nil || len(loaded) != 1 || loaded[0].Name != saved.Name {
			t.Fatalf("optional name not persisted: %+v, %v", loaded, err)
		}
	}
	task.Name = strings.Repeat("a", 501)
	request(t, h, "PUT", "/tasks/task", task, 400)
	// A nameless task can still be enabled, stopped and deleted.
	task.Name, task.Enabled = "", true
	request(t, h, "PUT", "/tasks/task", task, 200)
	if !svc.engine.State(task.ID).Running {
		t.Fatal("nameless task did not start")
	}
	request(t, h, "DELETE", "/tasks/task", nil, 200)
	if svc.engine.State(task.ID).Running {
		t.Fatal("deleted task still running")
	}
	request(t, h, "DELETE", "/accounts/account", nil, 200)
}
