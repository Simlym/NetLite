package dnsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type endpoints struct{ Cloudflare, DNSPod, Tencent, AliDNS string }

func defaultEndpoints() endpoints {
	return endpoints{"https://api.cloudflare.com/client/v4", "https://dnsapi.cn", "https://dnspod.tencentcloudapi.com", "https://alidns.aliyuncs.com"}
}

type providerAPI struct {
	task      Credentials
	client    *http.Client
	endpoints endpoints
	state     *Stats
	zones     map[string]string
}

type apiError struct{ code string }

func (e apiError) Error() string {
	return "DNS 服务商拒绝请求，请检查账号权限、域名及 TTL"
}
func errorCode(err error) string {
	var e apiError
	if errors.As(err, &e) {
		return e.code
	}
	return ""
}

var errNoZone = errors.New("DNS 账号中未找到匹配域名")
var errAmbiguous = errors.New("存在多条同名解析记录，无法自动选择；请保留一条目标记录")

// Never expose raw response bodies or transport errors: they can contain credentials.
func (p *providerAPI) do(req *http.Request, write bool, result any) error {
	p.state.ProviderRequests++
	if write {
		p.state.UpdateRequests++
	}
	p.state.LastRequest = time.Now().UTC()
	res, err := p.client.Do(req)
	if err != nil {
		p.state.NetworkError = true
		return fmt.Errorf("DNS 服务商连接失败")
	}
	defer res.Body.Close()
	p.state.HTTPStatus = res.StatusCode
	b, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return fmt.Errorf("DNS 服务商响应无效")
	}
	if json.Unmarshal(b, result) != nil {
		return fmt.Errorf("DNS 服务商响应无效")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// AliDNS returns structured API errors with HTTP 400, including missing zones.
		// Keep only an internal code; never return its Message or request URL.
		var status struct{ Code string }
		if json.Unmarshal(b, &status) == nil && status.Code != "" {
			return apiError{status.Code}
		}
		return fmt.Errorf("DNS 服务商请求失败（HTTP %d）", res.StatusCode)
	}
	return nil
}

// Probe exact zone names from longest to shortest. No public-suffix guess is used,
// so delegated subzones and names such as example.co.uk can be handled correctly.
func candidates(domain string) []string {
	parts := strings.Split(strings.TrimPrefix(domain, "*."), ".")
	var out []string
	for i := 0; i < len(parts)-1; i++ {
		out = append(out, strings.Join(parts[i:], "."))
	}
	return out
}
func subdomain(domain, zone string) string {
	if domain == zone {
		return "@"
	}
	return strings.TrimSuffix(domain, "."+zone)
}
func sameIP(a, b string) bool {
	x, y := net.ParseIP(a), net.ParseIP(b)
	return x != nil && y != nil && x.Equal(y)
}
func (p *providerAPI) sync(ctx context.Context, domain, typ, ip string) (bool, error) {
	switch p.task.Provider {
	case "cloudflare":
		return p.cloudflare(ctx, domain, typ, ip)
	case "dnspod":
		return p.dnspod(ctx, domain, typ, ip)
	case "tencentcloud":
		return p.tencent(ctx, domain, typ, ip)
	case "alidns":
		return p.alidns(ctx, domain, typ, ip)
	default:
		return false, fmt.Errorf("unsupported DNS provider")
	}
}
