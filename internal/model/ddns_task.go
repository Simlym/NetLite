package model

// DNSAccount contains display metadata only; secret material is stored separately.
type DNSAccount struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	CredentialID string `json:"credential_id"`
	HasSecret    bool   `json:"has_secret"`
}

// DNSCredential exists only while resolving engine credentials, never in Config.
type DNSCredential struct {
	Provider string
	ID       string `json:"-"`
	Secret   string `json:"-"`
}
type DDNSTask struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	AccountID   string   `json:"account_id"`
	IPv4Domains []string `json:"ipv4_domains"`
	IPv6Domains []string `json:"ipv6_domains"`
	IPv4URL     string   `json:"ipv4_url"`
	IPv6URL     string   `json:"ipv6_url"`
	Interval    int      `json:"interval_seconds"`
	TTL         string   `json:"ttl"`
	Enabled     bool     `json:"enabled"`
}
