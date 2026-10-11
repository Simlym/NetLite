package dnstask

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func (p *providerAPI) podCall(ctx context.Context, action string, values url.Values, write bool, out any) error {
	values.Set("login_token", p.task.CredentialID+","+p.task.Secret)
	values.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoints.DNSPod+"/"+action, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("invalid DNSPod endpoint")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var response json.RawMessage
	if err = p.do(req, write, &response); err != nil {
		return err
	}
	var status struct{ Status struct{ Code string } }
	if json.Unmarshal(response, &status) != nil || status.Status.Code == "" {
		return fmt.Errorf("DNSPod 响应无效")
	}
	if status.Status.Code != "1" {
		return apiError{status.Status.Code}
	}
	if out != nil && json.Unmarshal(response, out) != nil {
		return fmt.Errorf("DNSPod 响应无效")
	}
	return nil
}

func (p *providerAPI) podZone(ctx context.Context, domain string) (string, error) {
	for _, name := range candidates(domain) {
		if p.zones[name] != "" {
			return name, nil
		}
		var result struct{ Domain struct{ Name string } }
		err := p.podCall(ctx, "Domain.Info", url.Values{"domain": {name}}, false, &result)
		if errorCode(err) == "6" {
			continue
		} // domain does not exist
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(result.Domain.Name, name) {
			return "", fmt.Errorf("DNSPod 域名身份不匹配")
		}
		p.zones[name] = name
		return name, nil
	}
	return "", errNoZone
}

func (p *providerAPI) dnspod(ctx context.Context, domain, typ, ip string) (bool, error) {
	zone, err := p.podZone(ctx, domain)
	if err != nil {
		return false, err
	}
	sub := subdomain(domain, zone)
	values := url.Values{"domain": {zone}, "sub_domain": {sub}, "record_type": {typ}, "offset": {"0"}, "length": {"2"}}
	var result struct {
		Info struct {
			Total string `json:"record_total"`
		}
		Records []struct{ ID, Name, Type, Value, TTL, Line, Enabled string }
	}
	err = p.podCall(ctx, "Record.List", values, false, &result)
	// Code 10 is a successful lookup with no matching record, after verifying the zone.
	if errorCode(err) == "10" {
		err = nil
		result.Records = []struct{ ID, Name, Type, Value, TTL, Line, Enabled string }{}
	}
	if err != nil {
		return false, err
	}
	if result.Records == nil {
		return false, fmt.Errorf("DNSPod 记录列表不完整")
	}
	if len(result.Records) > 1 {
		return false, errAmbiguous
	}
	values = url.Values{"domain": {zone}, "sub_domain": {sub}, "record_type": {typ}, "value": {ip}, "ttl": {p.task.TTL}, "record_line": {"默认"}}
	action := "Record.Create"
	if len(result.Records) == 1 {
		r := result.Records[0]
		if r.ID == "" || !strings.EqualFold(r.Name, sub) || r.Type != typ || r.Enabled != "1" {
			return false, fmt.Errorf("DNSPod 记录身份不匹配或已停用")
		}
		if sameIP(r.Value, ip) {
			return false, nil
		}
		if r.Line == "" || r.TTL == "" {
			return false, fmt.Errorf("DNSPod 记录属性不完整")
		}
		action = "Record.Modify"
		values.Set("record_id", r.ID)
		values.Set("record_line", r.Line)
		values.Set("ttl", r.TTL)
	}
	err = p.podCall(ctx, action, values, true, nil)
	return err == nil, err
}
