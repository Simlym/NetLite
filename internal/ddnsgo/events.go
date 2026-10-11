package ddnsgo

import (
	"bytes"
	"encoding/json"
	"net"
	"sync"
	"time"
)

const eventPrefix = "NETLITE_DDNS_EVENT "

type bridgeEvent struct {
	Schema           int       `json:"schema"`
	Event            string    `json:"event"`
	At               time.Time `json:"at"`
	IPv4             string    `json:"ipv4"`
	IPv6             string    `json:"ipv6"`
	IPv4Result       string    `json:"ipv4_result"`
	IPv6Result       string    `json:"ipv6_result"`
	ProviderRequests int       `json:"provider_requests"`
	UpdateRequests   int       `json:"update_requests"`
	Action           string    `json:"action"`
	HTTPStatus       int       `json:"http_status"`
	NetworkError     bool      `json:"network_error"`
}
type eventWriter struct {
	mu        sync.Mutex
	line      []byte
	oversized bool
	child     *child
}

func (w *eventWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			if !w.oversized {
				w.child.accept(bytes.TrimSuffix(w.line, []byte{'\r'}))
			}
			w.line = w.line[:0]
			w.oversized = false
		} else if !w.oversized {
			if len(w.line) >= 16384 {
				w.oversized = true
				w.line = w.line[:0]
			} else {
				w.line = append(w.line, b)
			}
		}
	}
	return len(p), nil
}
func validResult(s string) bool {
	switch s {
	case "disabled", "ip_failed", "invalid_domain", "unchanged", "success", "failed":
		return true
	}
	return false
}
func safeIP(s string, v4 bool) string {
	ip := net.ParseIP(s)
	if ip == nil || (ip.To4() != nil) != v4 {
		return ""
	}
	return ip.String()
}
func (c *child) accept(line []byte) {
	if !bytes.HasPrefix(line, []byte(eventPrefix)) {
		return
	}
	var e bridgeEvent
	if json.Unmarshal(line[len(eventPrefix):], &e) != nil || e.Schema != 1 || e.At.IsZero() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := &c.state
	switch e.Event {
	case "check_started":
		s.LastCheck = e.At
		s.Checking = true
		s.ResultKnown = false
		s.IPv4Result = ""
		s.IPv6Result = ""
		s.ProviderRequests = 0
		s.UpdateRequests = 0
		s.HTTPStatus = 0
		s.NetworkError = false
		s.Message = "正在获取 IP 并检查解析记录"
	case "ip":
		s.IPv4 = safeIP(e.IPv4, true)
		s.IPv6 = safeIP(e.IPv6, false)
	case "provider_request":
		if e.ProviderRequests < 0 || e.UpdateRequests < 0 || e.UpdateRequests > e.ProviderRequests {
			return
		}
		s.ProviderRequests = e.ProviderRequests
		s.UpdateRequests = e.UpdateRequests
		s.LastRequest = e.At
		if e.Action == "update" || e.Action == "create" {
			s.Message = "已发起 DNS 更新请求"
		} else {
			s.Message = "正在查询 DNS 服务商"
		}
	case "provider_response":
		s.HTTPStatus = e.HTTPStatus
		s.NetworkError = e.NetworkError
	case "check_finished":
		if !validResult(e.IPv4Result) || !validResult(e.IPv6Result) || e.ProviderRequests < 0 || e.UpdateRequests < 0 || e.UpdateRequests > e.ProviderRequests {
			return
		}
		s.IPv4 = safeIP(e.IPv4, true)
		s.IPv6 = safeIP(e.IPv6, false)
		s.IPv4Result = e.IPv4Result
		s.IPv6Result = e.IPv6Result
		s.ProviderRequests = e.ProviderRequests
		s.UpdateRequests = e.UpdateRequests
		s.Checking = false
		s.ResultKnown = true
		s.LastFinished = e.At
		s.Message = "IP 未变化，无需更新"
		if s.ProviderRequests > 0 {
			s.Message = "已检查解析记录，无需更新"
		}
		if s.IPv4Result == "success" || s.IPv6Result == "success" {
			s.Message = "DNS 更新成功"
		}
		if s.IPv4Result == "failed" || s.IPv6Result == "failed" {
			s.Message = "服务商请求失败，请核对密钥权限和域名设置"
		}
		if s.IPv4Result == "ip_failed" || s.IPv6Result == "ip_failed" {
			s.Message = "公网 IP 获取失败"
		}
		if s.IPv4Result == "invalid_domain" || s.IPv6Result == "invalid_domain" {
			s.Message = "域名未被引擎识别"
		}
		if c.onFinished != nil {
			if err := c.onFinished(*s); err != nil {
				s.Message += "；检查记录保存失败"
			}
		}
	}
}
