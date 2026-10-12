package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"netlite/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Caddy struct {
	Binary, Data, Admin string
	Client              *http.Client
	ResolveCertificate  func(model.Certificate) (model.CertificateFiles, error)
	mu                  sync.Mutex
}

func New(binary, data, admin string) *Caddy {
	return &Caddy{Binary: binary, Data: data, Admin: admin, Client: &http.Client{Timeout: 15 * time.Second}}
}
func (c *Caddy) Render(cfg model.Config) ([]byte, error) {
	servers := map[string]any{}
	groups := map[int][]model.Route{}
	for _, r := range cfg.Routes {
		if r.Enabled {
			groups[r.Port] = append(groups[r.Port], r)
		}
	}
	for port, rs := range groups {
		sort.SliceStable(rs, func(i, j int) bool {
			return !strings.HasPrefix(rs[i].Domain, "*.") && strings.HasPrefix(rs[j].Domain, "*.")
		})
		routes := []any{}
		for _, r := range rs {
			u, e := url.Parse(r.Upstream)
			if e != nil {
				return nil, e
			}
			p := u.Port()
			if p == "" {
				p = "80"
				if u.Scheme == "https" {
					p = "443"
				}
			}
			handler := map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": net.JoinHostPort(u.Hostname(), p)}}}
			if u.Scheme == "https" {
				handler["transport"] = map[string]any{"protocol": "http", "tls": map[string]any{"server_name": u.Hostname()}}
				handler["headers"] = map[string]any{"request": map[string]any{"set": map[string]any{"Host": []string{u.Host}}}}
			}
			routes = append(routes, map[string]any{"match": []any{map[string]any{"host": []string{r.Domain}}}, "handle": []any{handler}, "terminal": true})
		}
		server := map[string]any{"listen": []string{":" + strconv.Itoa(port)}, "routes": routes, "automatic_https": map[string]any{"disable": true}}
		if rs[0].Scheme == "https" {
			tlsPolicies := []any{}
			for _, route := range rs {
				tlsPolicies = append(tlsPolicies, map[string]any{"match": map[string]any{"sni": []string{route.Domain}}, "certificate_selection": map[string]any{"any_tag": []string{route.CertificateID}}})
			}
			server["tls_connection_policies"] = tlsPolicies
		}
		servers["port_"+strconv.Itoa(port)] = server
	}
	used := map[string]bool{}
	for _, r := range cfg.Routes {
		if r.Enabled && r.Scheme == "https" {
			used[r.CertificateID] = true
		}
	}
	files := []any{}
	for _, policy := range cfg.Certificates {
		if !used[policy.ID] {
			continue
		}
		if c.ResolveCertificate == nil {
			return nil, fmt.Errorf("certificate resolver unavailable")
		}
		material, err := c.ResolveCertificate(policy)
		if err != nil {
			return nil, err
		}
		files = append(files, map[string]any{"certificate": material.CertificateFile, "key": material.PrivateKeyFile, "tags": []string{policy.ID}})
		delete(used, policy.ID)
	}
	if len(used) > 0 {
		return nil, fmt.Errorf("HTTPS route references missing certificate")
	}
	apps := map[string]any{"http": map[string]any{"servers": servers}}
	if len(files) > 0 {
		apps["tls"] = map[string]any{"certificates": map[string]any{"load_files": files}}
	}
	return json.MarshalIndent(map[string]any{"admin": map[string]any{"listen": c.Admin, "config": map[string]any{"persist": false}}, "apps": apps}, "", "  ")
}

func (c *Caddy) Modules(ctx context.Context) ([]string, error) {
	b, e := exec.CommandContext(ctx, c.Binary, "list-modules").Output()
	if e != nil {
		return nil, e
	}
	return strings.Fields(string(b)), nil
}
func (c *Caddy) Validate(ctx context.Context, cfg model.Config) error {
	b, e := c.Render(cfg)
	if e != nil {
		return e
	}
	cmd := exec.CommandContext(ctx, c.Binary, "validate", "--config", "-")
	cmd.Stdin = bytes.NewReader(b)
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("Caddy validation failed; check proxy configuration and certificate files")
	}
	return nil
}
func (c *Caddy) load(ctx context.Context, b []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+c.Admin+"/load", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("Caddy reload failed")
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != 200 {
		return fmt.Errorf("Caddy rejected configuration (%d)", res.StatusCode)
	}
	return nil
}
func (c *Caddy) Apply(ctx context.Context, cfg model.Config) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.Validate(ctx, cfg); err != nil {
		return err
	}
	b, err := c.Render(cfg)
	if err != nil {
		return err
	}
	checkpoint, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(c.Data, ".proxy-applied-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(checkpoint)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	previous, err := c.Applied()
	if err != nil {
		return err
	}
	if err = c.load(ctx, b); err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(c.Data, "proxy-applied.json")); err != nil {
		rollback, renderErr := c.Render(previous)
		if renderErr == nil {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			renderErr = c.load(rollbackCtx, rollback)
			cancel()
		}
		if renderErr != nil {
			return fmt.Errorf("proxy applied but checkpoint and rollback failed")
		}
		return fmt.Errorf("proxy checkpoint failed; previous configuration restored")
	}
	return nil
}
func (c *Caddy) Applied() (model.Config, error) {
	var cfg model.Config
	b, err := os.ReadFile(filepath.Join(c.Data, "proxy-applied.json"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(b, &cfg)
	return cfg, err
}
func (c *Caddy) ReloadCertificates(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg, err := c.Applied()
	if err != nil {
		return err
	}
	if len(cfg.Routes) == 0 {
		return nil
	}
	if err = c.Validate(ctx, cfg); err != nil {
		return err
	}
	b, err := c.Render(cfg)
	if err != nil {
		return err
	}
	return c.load(ctx, b)
}
func (c *Caddy) Status(ctx context.Context) (map[string]any, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+c.Admin+"/config/", nil)
	res, e := c.Client.Do(req)
	if e != nil {
		return map[string]any{"running": false}, nil
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	mods, _ := c.Modules(ctx)
	return map[string]any{"running": res.StatusCode == 200, "modules": mods, "engine": "caddy"}, nil
}
func (c *Caddy) Run(ctx context.Context) error {
	// Refuse to attach to another service on the selected private admin port.
	listener, e := net.Listen("tcp", c.Admin)
	if e != nil {
		return fmt.Errorf("private Caddy admin port occupied: %w", e)
	}
	listener.Close()
	cfg, e := c.Applied()
	if e != nil {
		return e
	}
	b, e := c.Render(cfg)
	if e != nil {
		return e
	}
	log, e := os.OpenFile(filepath.Join(c.Data, "caddy.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := exec.Command(c.Binary, "run", "--config", "-")
	cmd.Stdin = bytes.NewReader(b)
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e = <-done:
		return fmt.Errorf("Caddy exited: %v", e)
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		req, _ := http.NewRequestWithContext(shutdown, "POST", "http://"+c.Admin+"/stop", nil)
		if res, e := c.Client.Do(req); e == nil {
			res.Body.Close()
		}
		select {
		case <-done:
			return nil
		case <-shutdown.Done():
			cmd.Process.Kill()
			<-done
			return nil
		}
	}
}
