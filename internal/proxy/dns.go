package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"netlite/internal/model"
	"os"
	"path/filepath"
	"strings"
)

var DNSProviders = []string{"cloudflare", "dnspod", "tencentcloud", "alidns"}

func (c *Caddy) DNSCapabilities(ctx context.Context) []string {
	mods, e := c.Modules(ctx)
	if e != nil {
		return []string{}
	}
	out := []string{}
	for _, p := range DNSProviders {
		for _, m := range mods {
			if m == "dns.providers."+p {
				out = append(out, p)
				break
			}
		}
	}
	return out
}
func (c *Caddy) dnsProvider(x model.Certificate) (map[string]any, error) {
	if x.AccountID == "" {
		return map[string]any{"name": x.Provider, "api_token": "{env." + x.TokenEnv + "}"}, nil
	}
	if c.ResolveAccount == nil {
		return nil, fmt.Errorf("DNS account resolver unavailable")
	}
	a, e := c.ResolveAccount(x.AccountID)
	if e != nil {
		return nil, fmt.Errorf("证书 %s 的 DNS 账号不可用", x.ID)
	}
	ref := func(field string) string { return "{netlite.dns." + x.AccountID + "." + a.Provider + "." + field + "}" }
	out := map[string]any{"name": a.Provider}
	switch a.Provider {
	case "cloudflare":
		out["api_token"] = ref("secret")
	case "dnspod":
		out["api_token"] = ref("auth")
	case "tencentcloud":
		out["secretid"] = ref("id")
		out["secretkey"] = ref("secret")
	case "alidns":
		out["access_key_id"] = ref("id")
		out["access_key_secret"] = ref("secret")
	default:
		return nil, fmt.Errorf("证书引擎不支持此 DNS 服务商")
	}
	return out, nil
}

// Expand shared credentials only into stdin/Admin payloads. Persist references,
// so neither configuration previews nor restart checkpoints contain account secrets.
func (c *Caddy) runtimeConfig(b []byte) ([]byte, error) {
	var v any
	if e := json.Unmarshal(b, &v); e != nil {
		return nil, e
	}
	cache := map[string]model.DNSCredential{}
	var walk func(any) (any, error)
	walk = func(v any) (any, error) {
		switch x := v.(type) {
		case map[string]any:
			for k, a := range x {
				r, e := walk(a)
				if e != nil {
					return nil, e
				}
				x[k] = r
			}
		case []any:
			for i, a := range x {
				r, e := walk(a)
				if e != nil {
					return nil, e
				}
				x[i] = r
			}
		case string:
			if strings.HasPrefix(x, "{netlite.dns.") && strings.HasSuffix(x, "}") {
				parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(x, "{netlite.dns."), "}"), ".")
				if len(parts) != 3 || c.ResolveAccount == nil {
					return nil, fmt.Errorf("invalid DNS account reference")
				}
				id, provider, field := parts[0], parts[1], parts[2]
				a, ok := cache[id]
				if !ok {
					var e error
					a, e = c.ResolveAccount(id)
					if e != nil {
						return nil, fmt.Errorf("已应用证书引用的 DNS 账号不可用")
					}
					cache[id] = a
				}
				if a.Provider != provider || a.Secret == "" {
					return nil, fmt.Errorf("DNS 账号类型或凭据与已应用证书不匹配")
				}
				switch field {
				case "id":
					return a.ID, nil
				case "secret":
					return a.Secret, nil
				case "auth":
					return a.ID + "," + a.Secret, nil
				default:
					return nil, fmt.Errorf("invalid DNS credential field")
				}
			}
		}
		return v, nil
	}
	v, e := walk(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func (c *Caddy) AccountReferenced(id string) bool {
	b, e := os.ReadFile(filepath.Join(c.Data, "applied.json"))
	if os.IsNotExist(e) {
		return false
	}
	if e != nil {
		return true
	}
	return bytes.Contains(b, []byte("{netlite.dns."+id+"."))
}
