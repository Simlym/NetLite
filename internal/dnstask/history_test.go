package dnstask

import (
	"context"
	"encoding/json"
	"netlite/internal/store"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryPersistenceRetentionAndDeletion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := OpenService(context.Background(), db, filepath.Join(dir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`INSERT INTO dns_tasks VALUES('task','{"id":"task","enabled":false}')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		state := State{Running: true, ResultKnown: true, IPv4: "192.0.2.1", IPv4Result: "success", IPv6Result: "disabled", LastFinished: time.Unix(int64(i+1), 0), Message: "DNS 更新成功", UpdateRequests: 1}
		if err = svc.saveResult("task", state); err != nil {
			t.Fatal(err)
		}
	}
	db.DB.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	svc, err = OpenService(context.Background(), db, filepath.Join(dir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.history("task", 100)
	if err != nil || len(history) != 100 || history[0].LastFinished.Unix() != 105 || history[99].LastFinished.Unix() != 6 || history[0].Running {
		t.Fatalf("history: %v %v", history, err)
	}
	var response struct {
		Tasks []taskView `json:"tasks"`
	}
	json.Unmarshal(request(t, svc.Router(), "GET", "/", nil, 200), &response)
	if len(response.Tasks) != 1 || response.Tasks[0].State.IPv4 != "192.0.2.1" || response.Tasks[0].State.Running {
		t.Fatal("state not restored")
	}
	request(t, svc.Router(), "GET", "/tasks/task/history", nil, 200)
	request(t, svc.Router(), "GET", "/tasks/missing/history", nil, 404)
	request(t, svc.Router(), "DELETE", "/tasks/task", nil, 200)
	history, err = svc.history("task", 100)
	if err != nil || len(history) != 0 {
		t.Fatal("history not removed")
	}
}
