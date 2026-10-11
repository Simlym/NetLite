package dnstask

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (p *providerAPI) aliCall(ctx context.Context, action string, values url.Values, write bool, out any) error {
	values.Set("Action", action)
	if err := signAli(values, p.task.CredentialID, p.task.Secret, time.Now()); err != nil { return fmt.Errorf("cannot sign AliDNS request") }
	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoints.AliDNS+"/?"+values.Encode(), nil)
	if err != nil { return fmt.Errorf("invalid AliDNS endpoint") }
	var response json.RawMessage
	if err = p.do(req, write, &response); err != nil { return err }
	var status struct { Code, RequestID string }
	if json.Unmarshal(response, &status) != nil { return fmt.Errorf("阿里云响应无效") }
	if status.Code != "" { return apiError{status.Code} }
	if status.RequestID == "" { return fmt.Errorf("阿里云响应无效") }
	if out != nil && json.Unmarshal(response, out) != nil { return fmt.Errorf("阿里云响应无效") }
	return nil
}

func (p *providerAPI) aliZone(ctx context.Context, domain string) (string, error) {
	for _, name := range candidates(domain) {
		if p.zones[name] != "" { return name, nil }
		var result struct { DomainName string }
		err := p.aliCall(ctx, "DescribeDomainInfo", url.Values{"DomainName": {name}}, false, &result)
		if errorCode(err) == "InvalidDomainName.NoExist" || errorCode(err) == "InvalidDomainName.NotFound" { continue }
		if err != nil { return "", err }
		if !strings.EqualFold(result.DomainName, name) { return "", fmt.Errorf("阿里云域名身份不匹配") }
		p.zones[name] = name
		return name, nil
	}
	return "", errNoZone
}

func (p *providerAPI) alidns(ctx context.Context, domain, typ, ip string) (bool, error) {
	zone, err := p.aliZone(ctx, domain)
	if err != nil { return false, err }
	sub := subdomain(domain, zone)
	var result struct {
		TotalCount int
		DomainRecords struct { Record []struct { RecordID, DomainName, RR, Type, Value, Line, Status string; TTL int } }
	}
	err = p.aliCall(ctx, "DescribeSubDomainRecords", url.Values{"DomainName": {zone}, "SubDomain": {domain}, "Type": {typ}, "PageNumber": {"1"}, "PageSize": {"2"}}, false, &result)
	if err != nil { return false, err }
	if result.DomainRecords.Record == nil { return false, fmt.Errorf("阿里云记录列表不完整") }
	if result.TotalCount > 1 || len(result.DomainRecords.Record) > 1 { return false, errAmbiguous }
	if result.TotalCount != len(result.DomainRecords.Record) { return false, fmt.Errorf("阿里云记录列表不完整") }
	values := url.Values{"DomainName": {zone}, "RR": {sub}, "Type": {typ}, "Value": {ip}, "TTL": {p.task.TTL}, "Line": {"default"}}
	action := "AddDomainRecord"
	if len(result.DomainRecords.Record) == 1 {
		r := result.DomainRecords.Record[0]
		if r.RecordID == "" || !strings.EqualFold(r.DomainName, zone) || !strings.EqualFold(r.RR, sub) || r.Type != typ || r.Status != "ENABLE" { return false, fmt.Errorf("阿里云记录身份不匹配或已停用") }
		if sameIP(r.Value, ip) { return false, nil }
		if r.TTL <= 0 || r.Line == "" { return false, fmt.Errorf("阿里云记录属性不完整") }
		action = "UpdateDomainRecord"
		values.Del("DomainName")
		values.Set("RecordId", r.RecordID)
		values.Set("TTL", strconv.Itoa(r.TTL))
		values.Set("Line", r.Line)
	}
	var response struct { RecordID string }
	err = p.aliCall(ctx, action, values, true, &response)
	if err == nil && response.RecordID == "" { err = fmt.Errorf("阿里云未确认记录更新") }
	return err == nil, err
}
