//go:build linux

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"netlite/internal/auth"
	"netlite/internal/store"
	frontend "netlite/web"
)

func socketTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestGatewaySocketOwnershipAndCleanup(t *testing.T) {
	dir := socketTestDir(t)
	path := filepath.Join(dir, "netlite.sock")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenGateway(path, 0600, -1); err == nil {
		t.Fatal("overwrote regular file")
	}
	os.Remove(path)
	if _, err := listenGateway(path, 0666, -1); err == nil {
		t.Fatal("world-writable Socket accepted")
	}
	l, err := listenGateway(path, 0600, -1)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Stat(path)
	if err != nil || file.Mode().Perm() != 0600 {
		t.Fatal(file, err)
	}
	if _, err := listenGateway(path, 0600, -1); err == nil {
		t.Fatal("replaced active Socket")
	}
	l.Close()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("Socket remained after close", err)
	}
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	stale.Close()
	l, err = listenGateway(path, 0660, os.Getgid())
	if err != nil {
		t.Fatal("stale Socket recovery failed", err)
	}
	os.Remove(path)
	os.WriteFile(path, []byte("replacement"), 0600)
	l.Close()
	if content, err := os.ReadFile(path); err != nil || string(content) != "replacement" {
		t.Fatal("close removed replacement", err)
	}
	link := filepath.Join(dir, "link.sock")
	os.Symlink(path, link)
	if _, err := listenGateway(link, 0600, -1); err == nil {
		t.Fatal("followed symlink")
	}
}

// A subprocess exercises real startup, routing, DB initialization and shutdown.
func TestGatewayProcessHelper(t *testing.T) {
	if os.Getenv("NETLITE_GATEWAY_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			break
		}
	}
	flag.CommandLine = flag.NewFlagSet("netlite", flag.ExitOnError)
	if err := run(); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayProcessLifecycle(t *testing.T) {
	dir := socketTestDir(t)
	path := filepath.Join(dir, "netlite.sock")
	data := filepath.Join(dir, "data")
	os.Mkdir(data, 0700)
	// An old standalone HTTPS setting must not block platform-mode upgrades.
	db, err := store.Open(filepath.Join(data, "netlite.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.DB.Exec(`CREATE TABLE panel_settings(id INTEGER PRIMARY KEY,body TEXT NOT NULL); INSERT INTO panel_settings VALUES(1,'{"enabled":true,"domain":"expired.example","certificate_id":"missing"}')`)
	db.DB.Close()
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "process.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	process := exec.Command(exe, "-test.run=^TestGatewayProcessHelper$", "--", "--auth-mode", "fnos-gateway", "--gateway-socket", path, "--data", data, "--no-caddy")
	// Ordinary go test builds embed the source index. Use a tiny site fixture in
	// that case; -tags webembed exercises the actual production assets instead.
	index, err := io.ReadAll(mustOpenFrontend(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "/src/main.ts") {
		webDir := filepath.Join(dir, "web")
		os.MkdirAll(filepath.Join(webDir, "assets"), 0700)
		os.WriteFile(filepath.Join(webDir, "index.html"), []byte(`<script src="./assets/app.js"></script>`), 0600)
		os.WriteFile(filepath.Join(webDir, "assets/app.js"), []byte("console.log('test')"), 0600)
		process.Args = append(process.Args, "--web", webDir)
	}
	process.Env = append(os.Environ(), "NETLITE_GATEWAY_TEST_PROCESS=1")
	process.Stdout = log
	process.Stderr = log
	if err = process.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	processExited := false
	t.Cleanup(func() {
		if !processExited {
			process.Process.Kill()
			<-done
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err = gatewayHealth([]string{"--gateway-socket", path}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			content, _ := os.ReadFile(logPath)
			t.Fatalf("gateway did not start: %s", content)
		}
		time.Sleep(50 * time.Millisecond)
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, route, user, admin, csrf, origin, body string) (int, string) {
		t.Helper()
		r, _ := http.NewRequest(method, "http://nas.local:5001"+route, strings.NewReader(body))
		if user != "" {
			r.Header.Set("X-Trim-Userid", user)
			r.Header.Set("X-Trim-Username", "admin")
			r.Header.Set("X-Trim-Isadmin", admin)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		content, _ := io.ReadAll(response.Body)
		if route != "/healthz" && response.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("authenticated response may be cached", route)
		}
		return response.StatusCode, string(content)
	}
	for _, route := range []string{"/app/netlite/", "/app/netlite/api/config", "/app/netlite/api/auth/state"} {
		if status, _ := request("GET", route, "", "", "", "", ""); status != 401 {
			t.Fatal("anonymous app access", route, status)
		}
		if status, _ := request("GET", route, "1001", "false", "", "", ""); status != 403 {
			t.Fatal("ordinary user reached app", route, status)
		}
	}
	if status, body := request("GET", "/app/netlite/api/auth/state", "1000", "true", "", "", ""); status != 200 || !strings.Contains(body, `"mode":"fnos-gateway"`) {
		t.Fatal(status, body)
	}
	status, body := request("GET", "/app/netlite/api/auth/me", "1000", "true", "", "", "")
	if status != 200 {
		t.Fatal(body)
	}
	var principal auth.Principal
	if err = json.Unmarshal([]byte(body), &principal); err != nil || principal.CSRF == "" {
		t.Fatal(body, err)
	}
	status, body = request("GET", "/app/netlite/", "1000", "true", "", "", "")
	if status != 200 {
		t.Fatal("static root failed", status, body)
	}
	asset := regexp.MustCompile(`(?:src|href)="(\./assets/[^"]+)"`).FindStringSubmatch(body)
	if len(asset) != 2 {
		t.Fatal("embedded release assets are not relative", body)
	}
	if status, _ = request("GET", "/app/netlite/"+strings.TrimPrefix(asset[1], "./"), "1000", "true", "", "", ""); status != 200 {
		t.Fatal("asset route failed", status)
	}
	if status, _ = request("GET", "/api/config", "1000", "true", "", "", ""); status != 404 {
		t.Fatal("escaped gateway prefix", status)
	}
	if status, _ = request("POST", "/app/netlite/api/auth/login", "1000", "true", principal.CSRF, "", `{}`); status != 404 {
		t.Fatal("local login still enabled", status)
	}
	if status, _ = request("GET", "/app/netlite/api/panel", "1000", "true", "", "", ""); status != 403 {
		t.Fatal("standalone panel settings exposed", status)
	}
	status, config := request("GET", "/app/netlite/api/config", "1000", "true", "", "", "")
	if status != 200 {
		t.Fatal(config)
	}
	if status, _ = request("PUT", "/app/netlite/api/config", "1000", "true", "", "", config); status != 401 {
		t.Fatal("write without CSRF", status)
	}
	if status, _ = request("PUT", "/app/netlite/api/config", "1000", "true", principal.CSRF, "https://evil.example", config); status != 401 {
		t.Fatal("cross origin write", status)
	}
	if status, body = request("PUT", "/app/netlite/api/config", "1000", "true", principal.CSRF, "https://nas.local:5001", config); status != 200 {
		t.Fatal("authenticated write failed", status, body)
	}
	if _, err = os.Stat(filepath.Join(data, "admin-token")); !os.IsNotExist(err) {
		t.Fatal("gateway generated local credentials", err)
	}
	if err = process.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		processExited = true
		if err != nil {
			content, _ := os.ReadFile(logPath)
			t.Fatal(err, string(content))
		}
	case <-time.After(15 * time.Second):
		t.Fatal("gateway did not stop")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("shutdown left Socket", err)
	}
}

func mustOpenFrontend(t *testing.T) io.ReadCloser {
	t.Helper()
	file, err := frontend.FS.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}
