package proxy

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"netlite/internal/model"
	"syscall"
	"time"
)

type TargetResult struct {
	Reachable bool   `json:"reachable"`
	Status    int    `json:"http_status,omitempty"`
	Message   string `json:"message"`
}

// TestTarget sends only HEAD /, never follows redirects, and does not use environment proxies.
// Calling this function requires the same administrator authentication as editing routes.
func TestTarget(ctx context.Context, upstream string) (TargetResult, error) {
	cfg := model.Config{Routes: []model.Route{{ID: "probe", Domain: "localhost", Port: 18080, Scheme: "http", Upstream: upstream}}}
	if err := cfg.Validate(); err != nil {
		return TargetResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	transport := &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxResponseHeaderBytes: 32 << 10, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, upstream, nil)
	if err != nil {
		return TargetResult{}, err
	}
	response, err := client.Do(req)
	if err != nil {
		message := "连接失败，请检查目标地址、端口和服务监听地址"
		var dns *net.DNSError
		var unknownCA x509.UnknownAuthorityError
		var hostname x509.HostnameError
		var timeout net.Error
		switch {
		case errors.As(err, &dns):
			message = "目标域名无法解析，请检查域名或容器网络"
		case errors.Is(err, syscall.ECONNREFUSED):
			message = "目标拒绝连接，请检查服务是否启动、端口是否正确；bridge 模式的 127.0.0.1 指向容器自身"
		case errors.As(err, &unknownCA), errors.As(err, &hostname):
			message = "目标 HTTPS 证书不受信任或域名不匹配，请检查目标协议和证书"
		case errors.As(err, &timeout) && timeout.Timeout():
			message = "连接超时，请检查目标地址、服务监听地址和防火墙"
		}
		return TargetResult{Message: message}, nil
	}
	defer response.Body.Close()
	return TargetResult{Reachable: true, Status: response.StatusCode, Message: fmt.Sprintf("目标可连接，HTTP %d；仅验证从 NetLite 到目标的连接，未验证公网访问", response.StatusCode)}, nil
}
