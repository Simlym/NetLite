package dnstask

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(3*time.Second)
	for time.Now().Before(deadline) { if predicate() { return }; time.Sleep(5*time.Millisecond) }
	t.Fatal("task deadline exceeded")
}

func TestIPFailureNeverCallsProvider(t *testing.T) {
	ip := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("token=secret")) }))
	defer ip.Close()
	e := New()
	e.endpoints.Cloudflare = "http://127.0.0.1:1"
	task := sampleTask(); task.IPv4.URL = ip.URL
	s := e.check(context.Background(), task)
	if s.IPv4Result != "ip_failed" || s.ProviderRequests != 0 || s.UpdateRequests != 0 || !s.ResultKnown || s.LastFinished.IsZero() { t.Fatalf("incorrect failure: %+v", s) }
}

func TestAddressFamiliesAndIPResponseBounds(t *testing.T) {
	for _, body := range []string{"2001:db8::1", "192.0.2.1 extra", string(make([]byte, 129))} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		_, err := New().getIP(context.Background(), Source{URL: server.URL}, true)
		server.Close()
		if err == nil { t.Fatal("invalid IPv4 source accepted") }
	}
}

func TestCheckNowQueuesWithoutRestartAndStopCancels(t *testing.T) {
	entered := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		w.Write([]byte("invalid"))
	}))
	defer server.Close()
	e := New(); defer e.Close()
	task := sampleTask(); task.IPv4.URL = server.URL
	var finished atomic.Int32
	e.onFinished = func(string, State) error { finished.Add(1); return nil }
	if err := e.Start(context.Background(), task); err != nil { t.Fatal(err) }
	waitFor(t, func() bool { return finished.Load() == 1 && e.State(task.ID).ResultKnown })
	original := e.jobs[task.ID]
	if err := e.CheckNow(task.ID); err != nil { t.Fatal(err) }
	waitFor(t, func() bool { return finished.Load() == 2 })
	if e.jobs[task.ID] != original { t.Fatal("manual check replaced job") }
	e.Stop(task.ID)
	if e.State(task.ID).Running { t.Fatal("stopped job still running") }
	if e.CheckNow(task.ID) == nil { t.Fatal("queued check for stopped task") }
	select { case <-original.done: default: t.Fatal("stop did not join worker") }
}

func TestBoundedConcurrencyAndCancellation(t *testing.T) {
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1); defer active.Add(-1)
		for { old := peak.Load(); if n <= old || peak.CompareAndSwap(old, n) { break } }
		<-r.Context().Done()
	}))
	defer server.Close()
	e := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel(); defer e.Close()
	for i := 0; i < 8; i++ {
		task := sampleTask(); task.ID = string(rune('a'+i)); task.IPv4.URL = server.URL
		if err := e.Start(ctx, task); err != nil { t.Fatal(err) }
	}
	waitFor(t, func() bool { return active.Load() == 4 })
	if peak.Load() > 4 { t.Fatal("concurrency limit exceeded") }
	cancel()
	e.Close()
	waitFor(t, func() bool { return active.Load() == 0 })
}
