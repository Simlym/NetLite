// Package dnstask manages DDNS tasks using an in-process, bounded scheduler.
package dnstask

import (
	"fmt"
	"net/url"
	"netlite/internal/model"
	"regexp"
	"strconv"
	"strings"
)

// Task contains resolved credentials only in memory; never expose it through APIs.
type Task struct {
	ID, Name, Provider, CredentialID, Secret string
	IPv4, IPv6 Source
	Interval int
	TTL string
}

type Source struct {
	Domains []string
	URL string
}

var taskID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func Validate(t Task) error {
	if !taskID.MatchString(t.ID) || t.Interval < 60 || t.Interval > 86400 {
		return fmt.Errorf("invalid task ID or interval")
	}
	switch t.Provider {
	case "cloudflare", "dnspod", "tencentcloud", "alidns":
	default:
		return fmt.Errorf("unsupported DNS provider")
	}
	if t.Secret == "" || (t.Provider != "cloudflare" && t.CredentialID == "") {
		return fmt.Errorf("missing provider credentials")
	}
	ttl, err := strconv.Atoi(t.TTL)
	if err != nil || ttl < 1 || ttl > 86400 {
		return fmt.Errorf("invalid TTL")
	}
	if len(t.IPv4.Domains)+len(t.IPv6.Domains) == 0 || len(t.IPv4.Domains)+len(t.IPv6.Domains) > 100 {
		return fmt.Errorf("task needs 1..100 records")
	}
	for _, source := range []Source{t.IPv4, t.IPv6} {
		if len(source.Domains) == 0 { continue }
		u, err := url.Parse(source.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return fmt.Errorf("invalid IP source URL")
		}
		seen := map[string]bool{}
		for _, domain := range source.Domains {
			domain = strings.ToLower(domain)
			if !model.Domain(domain) || seen[domain] { return fmt.Errorf("invalid or duplicate DDNS domain") }
			seen[domain] = true
		}
	}
	return nil
}
