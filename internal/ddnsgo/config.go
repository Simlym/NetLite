// Package ddnsgo adapts the pinned upstream application without copying provider code.
package ddnsgo

import (
	"encoding/json"
	"fmt"
	"net/url"
	"netlite/internal/model"
	"regexp"
)

// Task is adapter input; native upstream configuration never enters business models.
// Credentials are resolved by the caller and must never be returned by management APIs.
type Task struct {
	ID           string
	Name         string
	Provider     string
	CredentialID string
	Secret       string
	IPv4         Source
	IPv6         Source
	Interval     int
	TTL          string
}
type Source struct {
	Domains []string
	URL     string
}

var taskID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type addressConfig struct {
	Enable  bool     `json:"enable"`
	GetType string   `json:"gettype"`
	URL     string   `json:"url"`
	Domains []string `json:"domains"`
}
type providerConfig struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	Secret string `json:"secret"`
}
type taskConfig struct {
	Name string         `json:"name"`
	IPv4 addressConfig  `json:"ipv4"`
	IPv6 addressConfig  `json:"ipv6"`
	DNS  providerConfig `json:"dns"`
	TTL  string         `json:"ttl"`
}

// Render produces JSON, a subset of YAML accepted by ddns-go's yaml.v3 reader.
// IP sources use HTTP(S), never upstream's shell-command mode.
func Render(t Task) ([]byte, error) {
	if !taskID.MatchString(t.ID) || t.Interval < 60 || t.Interval > 86400 {
		return nil, fmt.Errorf("invalid task ID or interval")
	}
	switch t.Provider {
	case "cloudflare", "dnspod", "tencentcloud", "alidns":
	default:
		return nil, fmt.Errorf("provider not enabled in adapter")
	}
	if t.Secret == "" || (t.Provider != "cloudflare" && t.CredentialID == "") {
		return nil, fmt.Errorf("missing provider credentials")
	}
	source := func(s Source) (addressConfig, error) {
		a := addressConfig{Enable: len(s.Domains) > 0, GetType: "url", URL: s.URL, Domains: s.Domains}
		if !a.Enable {
			return a, nil
		}
		u, e := url.Parse(s.URL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return a, fmt.Errorf("invalid IP source URL")
		}
		for _, d := range s.Domains {
			if !model.Domain(d) {
				return a, fmt.Errorf("invalid DDNS domain")
			}
		}
		return a, nil
	}
	v4, e := source(t.IPv4)
	if e != nil {
		return nil, e
	}
	v6, e := source(t.IPv6)
	if e != nil {
		return nil, e
	}
	if !v4.Enable && !v6.Enable {
		return nil, fmt.Errorf("task needs at least one enabled address family")
	}
	name := t.Name
	if name == "" {
		name = t.ID
	}
	config := struct {
		Tasks []taskConfig `json:"dnsconf"`
		NoWAN bool         `json:"notallowwanaccess"`
		Lang  string       `json:"lang"`
	}{[]taskConfig{{name, v4, v6, providerConfig{t.Provider, t.CredentialID, t.Secret}, t.TTL}}, true, "en"}
	return json.MarshalIndent(config, "", "  ")
}
