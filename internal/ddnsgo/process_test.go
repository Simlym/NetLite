package ddnsgo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Subprocess fixture checks the invocation without contacting a DNS provider.
func TestDDNSChild(t *testing.T) {
	if os.Getenv("NETLITE_DDNS_HELPER") != "1" {
		return
	}
	var path string
	noWeb := false
	for i, a := range os.Args {
		if a == "-c" && i+1 < len(os.Args) {
			path = os.Args[i+1]
		}
		if a == "-noweb" {
			noWeb = true
		}
	}
	if !noWeb || path == "" {
		os.Exit(2)
	}
	if _, e := os.ReadFile(path); e != nil {
		os.Exit(3)
	}
	if e := os.WriteFile(filepath.Join(filepath.Dir(path), "ready"), []byte("ready"), 0600); e != nil {
		os.Exit(4)
	}
	if os.Getenv("NETLITE_DDNS_EVENTS") == "1" {
		fmt.Println(eventPrefix + `{"schema":1,"at":"2026-10-11T00:00:00Z","event":"check_finished","ipv4":"192.0.2.123","ipv4_result":"success","ipv6_result":"disabled","provider_requests":2,"update_requests":1}`)
	}
	for {
		time.Sleep(time.Second)
	}
}
func TestChildStatusPipeAndStoppedHistory(t *testing.T) {
	e := New("unused", t.TempDir())
	defer e.Close()
	e.command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		c := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestDDNSChild$", "--"}, args...)...)
		c.Env = append(os.Environ(), "NETLITE_DDNS_HELPER=1", "NETLITE_DDNS_EVENTS=1")
		return c
	}
	task := sampleTask()
	if err := e.Start(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return e.State(task.ID).ResultKnown })
	s := e.State(task.ID)
	if !s.Running || s.IPv4 != "192.0.2.123" || s.UpdateRequests != 1 {
		t.Fatalf("pipe did not report child events: %+v", s)
	}
	e.Stop(task.ID)
	s = e.State(task.ID)
	if s.Running || s.IPv4 != "192.0.2.123" || !s.ResultKnown {
		t.Fatal("stop lost last check status")
	}
}
func waitFor(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("child state deadline exceeded")
}
func TestLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := New("unused", t.TempDir())
	defer e.Close()
	e.command = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		c := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestDDNSChild$", "--"}, args...)...)
		c.Env = append(os.Environ(), "NETLITE_DDNS_HELPER=1")
		return c
	}
	task := sampleTask()
	if err := e.Start(ctx, task); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(e.dir, task.ID, "ready")
	waitFor(t, func() bool { _, err := os.Stat(ready); return err == nil })
	if s := e.State(task.ID); !s.Running || s.ResultKnown {
		t.Fatal("process state misrepresented DNS success")
	}
	old := e.children[task.ID]
	task.Interval = 600
	if err := e.Start(ctx, task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-old.done:
	default:
		t.Fatal("replacement left previous child running")
	}
	e.Stop(task.ID)
	if _, err := os.Stat(filepath.Join(e.dir, task.ID, "config.json")); !os.IsNotExist(err) {
		t.Fatal("stopped task retained generated plaintext configuration")
	}
	if e.State(task.ID).Running {
		t.Fatal("stop left child running")
	}
	if err := e.Start(ctx, task); err != nil {
		t.Fatal(err)
	}
	cancel()
	waitFor(t, func() bool { return !e.State(task.ID).Running })
}
func TestMissingExecutable(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "missing"), t.TempDir())
	defer e.Close()
	if err := e.Start(context.Background(), sampleTask()); err == nil {
		t.Fatal("missing executable accepted")
	}
}
