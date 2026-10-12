// Package dnsprovider shares DNS service APIs between DDNS and certificate validation.
package dnsprovider

import (
	"context"
	"fmt"
	"net/http"
	"netlite/internal/model"
	"time"
)

type Credentials struct{ Provider, CredentialID, Secret, TTL string }
type Stats struct {
	ProviderRequests, UpdateRequests, HTTPStatus int
	NetworkError                                 bool
	LastRequest                                  time.Time
}
type Client struct {
	api   providerAPI
	stats Stats
}

var Providers = []string{"cloudflare", "dnspod", "tencentcloud", "alidns"}

func New(a model.DNSCredential) *Client {
	c := &Client{}
	c.api = providerAPI{task: Credentials{Provider: a.Provider, CredentialID: a.ID, Secret: a.Secret}, client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, endpoints: defaultEndpoints(), state: &c.stats, zones: map[string]string{}}
	return c
}
func (c *Client) Stats() Stats { return c.stats }

// NewWithHTTP allows a caller to supply a transport without changing provider endpoints.
func NewWithHTTP(a model.DNSCredential, httpClient *http.Client) *Client {
	c := New(a)
	if httpClient != nil {
		c.api.client = httpClient
	}
	return c
}
func (c *Client) Sync(ctx context.Context, name, typ, value, ttl string) (bool, error) {
	if typ != "A" && typ != "AAAA" {
		return false, fmt.Errorf("DDNS supports A/AAAA only")
	}
	c.api.task.TTL = ttl
	return c.api.sync(ctx, name, typ, value)
}
