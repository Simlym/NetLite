package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const gatewayPrefix = "/app/netlite"

// The lifecycle probe uses the same executable and needs no curl dependency.
func gatewayHealth(args []string) error {
	flags := flag.NewFlagSet("gateway-health", flag.ContinueOnError)
	path := flags.String("gateway-socket", "", "gateway Unix Socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !filepath.IsAbs(*path) || flags.NArg() != 0 {
		return fmt.Errorf("gateway-health requires an absolute --gateway-socket")
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", *path)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get("http://localhost/healthz")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway health returned HTTP %d", response.StatusCode)
	}
	return nil
}

// Keep the public prefix intact at the gateway and strip it only at our router.
// Relative frontend assets resolve from the canonical trailing-slash URL.
func gatewayRoutes(next http.Handler) http.Handler {
	stripped := http.StripPrefix(gatewayPrefix, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == gatewayPrefix && (r.Method == "GET" || r.Method == "HEAD") {
			target := gatewayPrefix + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
			return
		}
		if !strings.HasPrefix(r.URL.Path, gatewayPrefix+"/") {
			http.NotFound(w, r)
			return
		}
		stripped.ServeHTTP(w, r)
	})
}

type socketListener struct {
	net.Listener
	path string
	file os.FileInfo
}

func (l *socketListener) Close() error {
	err := l.Listener.Close()
	// Never remove a file that has replaced our Socket since it was opened.
	if current, e := os.Lstat(l.path); e == nil && os.SameFile(l.file, current) {
		if e = os.Remove(l.path); err == nil {
			err = e
		}
	}
	return err
}

func listenGateway(path string, mode os.FileMode, group int) (net.Listener, error) {
	if !filepath.IsAbs(path) || filepath.Ext(path) != ".sock" {
		return nil, fmt.Errorf("gateway Socket must be an absolute .sock path")
	}
	if mode != 0600 && mode != 0660 {
		return nil, fmt.Errorf("gateway Socket permissions must be 0600 or 0660")
	}
	if group < -1 {
		return nil, fmt.Errorf("gateway Socket group must be -1 or a nonnegative GID")
	}
	if file, err := os.Lstat(path); err == nil {
		if file.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("gateway Socket path is occupied by a non-Socket file")
		}
		conn, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			conn.Close()
			return nil, fmt.Errorf("gateway Socket is already in use")
		}
		// Only a confirmed stale Socket can be replaced, not permission errors.
		if !isStaleSocket(err) {
			return nil, fmt.Errorf("cannot establish whether gateway Socket is stale: %w", err)
		}
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(file, current) {
			return nil, fmt.Errorf("gateway Socket changed during startup")
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// Create and chmod behind a private directory before publishing the Socket.
	// This avoids a world-writable window when started without a shell umask.
	privateDir, err := os.MkdirTemp(filepath.Dir(path), ".nl-")
	if err != nil {
		return nil, err
	}
	defer os.Remove(privateDir)
	privatePath := filepath.Join(privateDir, "s")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: privatePath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	l.SetUnlinkOnClose(false)
	file, err := os.Lstat(privatePath)
	if err != nil {
		l.Close()
		return nil, err
	}
	listener := &socketListener{Listener: l, path: privatePath, file: file}
	if group >= 0 {
		if err = os.Chown(privatePath, -1, group); err != nil {
			listener.Close()
			return nil, err
		}
	}
	if err = os.Chmod(privatePath, mode); err != nil {
		listener.Close()
		return nil, err
	}
	// Linking publishes atomically without overwriting a concurrent starter.
	if err = os.Link(privatePath, path); err != nil {
		listener.Close()
		return nil, err
	}
	if err = os.Remove(privatePath); err != nil {
		listener.Close()
		os.Remove(path)
		return nil, err
	}
	listener.path = path
	return listener, nil
}
