package model

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

type Config struct {
	Revision     int64         `json:"revision"`
	Routes       []Route       `json:"routes"`
	Certificates []Certificate `json:"certificates"`
}
type Route struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Domain        string `json:"domain"`
	Port          int    `json:"port"`
	Scheme        string `json:"scheme"`
	Upstream      string `json:"upstream"`
	CertificateID string `json:"certificate_id"`
	Enabled       bool   `json:"enabled"`
}
type Certificate struct {
	ID        string   `json:"id"`
	Domains   []string `json:"domains"`
	Mode      string   `json:"mode"`
	Email     string   `json:"email"`
	Provider  string   `json:"provider"`
	TokenEnv  string   `json:"token_env"`
	AccountID string   `json:"account_id,omitempty"`
	Staging   bool     `json:"staging"`
}
var ident = regexp.MustCompile("^[a-zA-Z0-9_-]{1,64}$")
var env = regexp.MustCompile("^[A-Z][A-Z0-9_]{0,100}$")
var label = regexp.MustCompile("^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$")

func Domain(s string) bool {
	if len(s) > 253 || s != strings.ToLower(s) {
		return false
	}
	s = strings.TrimPrefix(s, "*.")
	if net.ParseIP(s) != nil {
		return false
	}
	for _, l := range strings.Split(s, ".") {
		if !label.MatchString(l) {
			return false
		}
	}
	return s != ""
}
func Covers(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") || strings.HasPrefix(host, "*.") {
		return false
	}
	suffix := pattern[1:]
	return strings.HasSuffix(host, suffix) && !strings.Contains(strings.TrimSuffix(host, suffix), ".")
}
func (c Config) Validate(reserved ...int) error {
	if len(c.Routes) > 200 || len(c.Certificates) > 100 {
		return fmt.Errorf("too many entries")
	}
	seen := map[string]bool{}
	certs := map[string]Certificate{}
	subjects := map[string]bool{}
	for _, x := range c.Certificates {
		if !ident.MatchString(x.ID) || seen["c"+x.ID] {
			return fmt.Errorf("invalid/duplicate certificate id")
		}
		seen["c"+x.ID] = true
		if x.Mode != "internal" && x.Mode != "acme" && x.Mode != "dns" {
			return fmt.Errorf("invalid certificate mode")
		}
		if len(x.Domains) == 0 || len(x.Domains) > 50 {
			return fmt.Errorf("certificate domains required")
		}
		for _, d := range x.Domains {
			if !Domain(d) || subjects[d] {
				return fmt.Errorf("invalid/duplicate certificate domain %q", d)
			}
			subjects[d] = true
			if strings.HasPrefix(d, "*.") && x.Mode != "dns" {
				return fmt.Errorf("wildcard requires DNS-01")
			}
		}
		if x.Mode == "dns" {
			if x.AccountID != "" {
				if !ident.MatchString(x.AccountID) || x.Provider != "" || x.TokenEnv != "" {
					return fmt.Errorf("DNS account reference cannot be mixed with legacy provider/token_env")
				}
			} else if x.Provider != "cloudflare" || !env.MatchString(x.TokenEnv) {
				return fmt.Errorf("DNS-01 requires a DNS account or legacy cloudflare token_env")
			}
		} else if x.AccountID != "" {
			return fmt.Errorf("DNS account is only used for DNS-01")
		}
		if strings.ContainsAny(x.Email, "\r\n") {
			return fmt.Errorf("invalid email")
		}
		certs[x.ID] = x
	}
	ports := map[int]string{}
	routes := map[string]bool{}
	for _, r := range c.Routes {
		if !ident.MatchString(r.ID) || seen["r"+r.ID] {
			return fmt.Errorf("invalid/duplicate route id")
		}
		seen["r"+r.ID] = true
		if !Domain(r.Domain) || (r.Scheme != "http" && r.Scheme != "https") {
			return fmt.Errorf("invalid route domain/scheme")
		}
		if r.Port < 1024 || r.Port > 65535 {
			return fmt.Errorf("use unprivileged port 1024..65535")
		}
		for _, p := range reserved {
			if r.Port == p {
				return fmt.Errorf("port %d reserved", p)
			}
		}
		u, e := url.Parse(r.Upstream)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("upstream must be http(s)://host[:port] without path or credentials")
		}
		if r.Scheme == "https" {
			x, ok := certs[r.CertificateID]
			covered := false
			for _, d := range x.Domains {
				covered = covered || Covers(d, r.Domain)
			}
			if !ok || !covered {
				return fmt.Errorf("HTTPS route needs a covering certificate")
			}
		}
		if r.Enabled {
			if old, ok := ports[r.Port]; ok && old != r.Scheme {
				return fmt.Errorf("HTTP and HTTPS cannot share port")
			}
			ports[r.Port] = r.Scheme
			key := fmt.Sprintf("%s:%d", r.Domain, r.Port)
			if routes[key] {
				return fmt.Errorf("duplicate domain/port")
			}
			routes[key] = true
		}
	}
	return nil
}

