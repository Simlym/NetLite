package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (p *providerAPI) tcCall(ctx context.Context, action string, values any, write bool, out any) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoints.Tencent+"/", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid Tencent endpoint")
	}
	signTencent(req, p.task.CredentialID, p.task.Secret, action, data, time.Now())
	var envelope struct{ Response json.RawMessage }
	if err = p.do(req, write, &envelope); err != nil {
		return err
	}
	var status struct {
		RequestID string `json:"RequestId"`
		Error     *struct{ Code string }
	}
	if json.Unmarshal(envelope.Response, &status) != nil {
		return fmt.Errorf("腾讯云响应无效")
	}
	if status.Error != nil {
		return apiError{status.Error.Code}
	}
	if status.RequestID == "" {
		return fmt.Errorf("腾讯云响应无效")
	}
	if out != nil && json.Unmarshal(envelope.Response, out) != nil {
		return fmt.Errorf("腾讯云响应无效")
	}
	return nil
}

func (p *providerAPI) tcZone(ctx context.Context, domain string) (string, error) {
	for _, name := range candidates(domain) {
		if p.zones[name] != "" {
			return name, nil
		}
		var result struct{ DomainInfo struct{ Domain string } }
		err := p.tcCall(ctx, "DescribeDomain", map[string]string{"Domain": name}, false, &result)
		if errorCode(err) == "ResourceNotFound.NoDataOfDomain" {
			continue
		}
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(result.DomainInfo.Domain, name) {
			return "", fmt.Errorf("腾讯云域名身份不匹配")
		}
		p.zones[name] = name
		return name, nil
	}
	return "", errNoZone
}

func (p *providerAPI) tencent(ctx context.Context, domain, typ, ip string) (bool, error) {
	zone, err := p.tcZone(ctx, domain)
	if err != nil {
		return false, err
	}
	sub := subdomain(domain, zone)
	var result struct {
		RecordCountInfo struct{ TotalCount int }
		RecordList      []struct {
			RecordID                                int64 `json:"RecordId"`
			Name, Type, Value, Line, LineID, Status string
			TTL                                     int
		}
	}
	err = p.tcCall(ctx, "DescribeRecordList", map[string]any{"Domain": zone, "Subdomain": sub, "RecordType": typ, "Offset": 0, "Limit": 2}, false, &result)
	noRecords := errorCode(err) == "ResourceNotFound.NoDataOfRecord"
	if noRecords {
		err = nil
	}
	if err != nil {
		return false, err
	}
	if result.RecordList == nil && !noRecords {
		return false, fmt.Errorf("腾讯云记录列表不完整")
	}
	if len(result.RecordList) > 1 || result.RecordCountInfo.TotalCount > 1 {
		return false, errAmbiguous
	}
	if result.RecordCountInfo.TotalCount != len(result.RecordList) {
		return false, fmt.Errorf("腾讯云记录列表不完整")
	}
	ttl, _ := strconv.Atoi(p.task.TTL)
	values := map[string]any{"Domain": zone, "SubDomain": sub, "RecordType": typ, "RecordLine": "默认", "Value": ip, "TTL": ttl}
	action := "CreateRecord"
	if len(result.RecordList) == 1 {
		r := result.RecordList[0]
		if r.RecordID <= 0 || !strings.EqualFold(r.Name, sub) || r.Type != typ || r.Status != "ENABLE" {
			return false, fmt.Errorf("腾讯云记录身份不匹配或已停用")
		}
		if sameIP(r.Value, ip) {
			return false, nil
		}
		if r.TTL <= 0 || r.Line == "" {
			return false, fmt.Errorf("腾讯云记录属性不完整")
		}
		action = "ModifyRecord"
		values["RecordId"], values["TTL"], values["RecordLine"] = r.RecordID, r.TTL, r.Line
		if r.LineID != "" {
			values["RecordLineId"] = r.LineID
		}
	}
	err = p.tcCall(ctx, action, values, true, nil)
	return err == nil, err
}
