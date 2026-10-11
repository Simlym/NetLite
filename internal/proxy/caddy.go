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
	"time"
)

type Caddy struct {
	Binary, Data, Admin string
	HTTPPort, HTTPSPort int
	Client              *http.Client
	ResolveAccount      func(string) (model.DNSCredential, error)
}

func New(binary, data, admin string) *Caddy {
	return &Caddy{Binary: binary, Data: data, Admin: admin, HTTPPort: 18080, HTTPSPort: 18443, Client: &http.Client{Timeout: 15 * time.Second}}
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
			server["tls_connection_policies"] = []any{map[string]any{}}
		}
		servers["port_"+strconv.Itoa(port)] = server
	}
	policies := []any{}
	domains := []string{}
	for _, x := range cfg.Certificates {
		issuer := map[string]any{"module": "internal"}
		if x.Mode != "internal" {
			ca := "https://acme-v02.api.letsencrypt.org/directory"
			if x.Staging {
				ca = "https://acme-staging-v02.api.letsencrypt.org/directory"
			}
			issuer = map[string]any{"module": "acme", "email": x.Email, "ca": ca}
			if x.Mode == "dns" {
				provider, err := c.dnsProvider(x)
				if err != nil {
					return nil, err
				}
				issuer["challenges"] = map[string]any{"dns": map[string]any{"provider": provider}}
			}
		}
		domains = append(domains, x.Domains...)
		policies = append(policies, map[string]any{"subjects": x.Domains, "issuers": []any{issuer}})
	}
	apps := map[string]any{"http": map[string]any{"http_port": c.HTTPPort, "https_port": c.HTTPSPort, "servers": servers}}
	if len(domains) > 0 {
		apps["tls"] = map[string]any{"certificates": map[string]any{"automate": domains}, "automation": map[string]any{"policies": policies}}
	}
	// No trust-store changes are needed for NetLite's internal test certificates.
	for _, x := range cfg.Certificates {
		if x.Mode == "internal" {
			apps["pki"] = map[string]any{"certificate_authorities": map[string]any{"local": map[string]any{"install_trust": false}}}
			break
		}
	}
	return json.MarshalIndent(map[string]any{"admin": map[string]any{"listen": c.Admin, "config": map[string]any{"persist": false}}, "storage": map[string]any{"module": "file_system", "root": filepath.Join(c.Data, "certificates")}, "apps": apps}, "", "  ")
}
func (c *Caddy) Modules(ctx context.Context) ([]string, error) {
	b, e := exec.CommandContext(ctx, c.Binary, "list-modules").Output()
	if e != nil {
		return nil, e
	}
	return strings.Fields(string(b)), nil
}
func (c *Caddy) Validate(ctx context.Context, cfg model.Config) error {
	mods, e := c.Modules(ctx)
	if e != nil {
		return fmt.Errorf("Caddy unavailable: %w", e)
	}
	for _, x := range cfg.Certificates {
		if x.Mode == "dns" {
			provider, e := c.dnsProvider(x)
			if e != nil {
				return e
			}
			found := false
			for _, m := range mods {
				found = found || m == "dns.providers."+provider["name"].(string)
			}
			if !found {
				return fmt.Errorf("Caddy missing dns.providers.%s; build with xcaddy", provider["name"])
			}
			if x.AccountID == "" && os.Getenv(x.TokenEnv) == "" {
				return fmt.Errorf("missing credential environment %s", x.TokenEnv)
			}
		}
	}
	b, e := c.Render(cfg)
	if e != nil {
		return e
	}
	b, e = c.runtimeConfig(b)
	if e != nil {
		return e
	}
	cmd := exec.CommandContext(ctx, c.Binary, "validate", "--config", "-")
	cmd.Stdin = bytes.NewReader(b)
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("Caddy validation failed; check configuration and plugins")
	}
	return nil
}
func (c *Caddy) Apply(ctx context.Context, cfg model.Config) error {
	if e := c.Validate(ctx, cfg); e != nil {
		return e
	}
	b, e := c.Render(cfg)
	if e != nil {
		return e
	}
	runtime, e := c.runtimeConfig(b)
	if e != nil {
		return e
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+c.Admin+"/load", bytes.NewReader(runtime))
	req.Header.Set("Content-Type", "application/json")
	res, e := c.Client.Do(req)
	if e != nil {
		return fmt.Errorf("Caddy reload failed: %w", e)
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != 200 {
		return fmt.Errorf("Caddy rejected configuration (%d)", res.StatusCode)
	}
	// Keep last applied config distinct from the saved draft; restart never activates a draft.
	tmp := filepath.Join(c.Data, "applied.json.tmp")
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return fmt.Errorf("active config applied but checkpoint failed: %w", e)
	}
	return os.Rename(tmp, filepath.Join(c.Data, "applied.json"))
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
	config := filepath.Join(c.Data, "applied.json")
	if _, e = os.Stat(config); os.IsNotExist(e) {
		b, _ := c.Render(model.Config{})
		config = filepath.Join(c.Data, "bootstrap.json")
		if e = os.WriteFile(config, b, 0600); e != nil {
			return e
		}
	}
	log, e := os.OpenFile(filepath.Join(c.Data, "caddy.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	b, e := os.ReadFile(config)
	if e != nil {
		return e
	}
	b, e = c.runtimeConfig(b)
	if e != nil {
		return e
	}
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
