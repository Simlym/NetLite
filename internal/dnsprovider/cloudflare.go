package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type cfEnvelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Info    struct {
		Total int `json:"total_count"`
	} `json:"result_info"`
}

func (p *providerAPI) cfCall(ctx context.Context, method, path string, body any, out any) (int, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, p.endpoints.Cloudflare+path, bytes.NewReader(data))
	if err != nil {
		return 0, fmt.Errorf("invalid Cloudflare endpoint")
	}
	req.Header.Set("Authorization", "Bearer "+p.task.Secret)
	req.Header.Set("Content-Type", "application/json")
	var envelope cfEnvelope
	if err := p.do(req, method != "GET", &envelope); err != nil {
		return 0, err
	}
	if !envelope.Success {
		return 0, apiError{"cloudflare"}
	}
	if out != nil && (len(envelope.Result) == 0 || string(envelope.Result) == "null" || json.Unmarshal(envelope.Result, out) != nil) {
		return 0, fmt.Errorf("Cloudflare 响应无效")
	}
	return envelope.Info.Total, nil
}

func (p *providerAPI) cfZone(ctx context.Context, domain string) (string, error) {
	for _, name := range candidates(domain) {
		if id := p.zones[name]; id != "" {
			return id, nil
		}
		var zones []struct{ ID, Name, Status string }
		_, err := p.cfCall(ctx, "GET", "/zones?"+url.Values{"name": {name}, "status": {"active"}, "per_page": {"2"}}.Encode(), nil, &zones)
		if err != nil {
			return "", err
		}
		for _, zone := range zones {
			if strings.EqualFold(zone.Name, name) && zone.ID != "" && zone.Status == "active" {
				p.zones[name] = zone.ID
				return zone.ID, nil
			}
		}
	}
	return "", errNoZone
}

func (p *providerAPI) cloudflare(ctx context.Context, domain, typ, ip string) (bool, error) {
	zone, err := p.cfZone(ctx, domain)
	if err != nil {
		return false, err
	}
	path := "/zones/" + url.PathEscape(zone) + "/dns_records"
	var records []struct{ ID, Name, Type, Content string }
	total, err := p.cfCall(ctx, "GET", path+"?"+url.Values{"name": {domain}, "type": {typ}, "per_page": {"2"}}.Encode(), nil, &records)
	if err != nil {
		return false, err
	}
	if total > 1 || len(records) > 1 {
		return false, errAmbiguous
	}
	if len(records) == 1 {
		r := records[0]
		if r.ID == "" || !strings.EqualFold(r.Name, domain) || r.Type != typ {
			return false, fmt.Errorf("Cloudflare 记录身份不匹配")
		}
		if sameIP(r.Content, ip) {
			return false, nil
		}
		// PATCH only content; existing proxy status, TTL, comments and tags survive.
		_, err = p.cfCall(ctx, "PATCH", path+"/"+url.PathEscape(r.ID), map[string]string{"content": ip}, nil)
		return err == nil, err
	}
	ttl, _ := strconv.Atoi(p.task.TTL)
	if ttl != 1 && ttl < 60 {
		return false, fmt.Errorf("Cloudflare TTL 必须为 1（自动）或至少 60 秒")
	}
	_, err = p.cfCall(ctx, "POST", path, map[string]any{"name": domain, "type": typ, "content": ip, "ttl": ttl, "proxied": false}, nil)
	return err == nil, err
}
