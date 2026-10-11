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
	DDNS         []DDNS        `json:"ddns"`
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
	ID       string   `json:"id"`
	Domains  []string `json:"domains"`
	Mode     string   `json:"mode"`
	Email    string   `json:"email"`
	Provider string   `json:"provider"`
	TokenEnv string   `json:"token_env"`
	Staging  bool     `json:"staging"`
}
type DDNS struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	ZoneID   string `json:"zone_id"`
	RecordID string `json:"record_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IP       string `json:"ip"`
	TokenEnv string `json:"token_env"`
	Interval int    `json:"interval_seconds"`
	Enabled  bool   `json:"enabled"`
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
	if len(c.Routes) > 200 || len(c.Certificates) > 100 || len(c.DDNS) > 100 {
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
		if x.Mode == "dns" && (x.Provider != "cloudflare" || !env.MatchString(x.TokenEnv)) {
			return fmt.Errorf("DNS-01 currently supports cloudflare with token_env")
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
	for _, d := range c.DDNS {
		if !ident.MatchString(d.ID) || seen["d"+d.ID] {
			return fmt.Errorf("invalid/duplicate DDNS id")
		}
		seen["d"+d.ID] = true
		if d.Provider != "cloudflare" || !ident.MatchString(d.ZoneID) || !ident.MatchString(d.RecordID) || !Domain(d.Name) || strings.HasPrefix(d.Name, "*.") || !env.MatchString(d.TokenEnv) {
			return fmt.Errorf("invalid Cloudflare DDNS fields")
		}
		if d.Type != "A" && d.Type != "AAAA" {
			return fmt.Errorf("DDNS type must be A/AAAA")
		}
		if d.Interval < 60 || d.Interval > 86400 {
			return fmt.Errorf("interval must be 60..86400")
		}
		if d.IP != "" {
			ip := net.ParseIP(d.IP)
			if ip == nil || (d.Type == "A") != (ip.To4() != nil) {
				return fmt.Errorf("IP family mismatch")
			}
		}
	}
	return nil
}
