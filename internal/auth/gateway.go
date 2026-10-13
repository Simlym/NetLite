package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type gatewayConnectionKey struct{}

// GatewayConnContext marks connections accepted on a Unix Socket. HTTP headers
// alone must never opt a TCP request into platform authentication.
func GatewayConnContext(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, gatewayConnectionKey{}, conn.LocalAddr().Network() == "unix")
}

type Gateway struct {
	secret [32]byte
}

func NewGateway() (*Gateway, error) {
	g := &Gateway{}
	_, err := rand.Read(g.secret[:])
	return g, err
}

func singleHeader(r *http.Request, name string) string {
	values := r.Header.Values(name)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func (g *Gateway) csrf(p Principal) string {
	mac := hmac.New(sha256.New, g.secret[:])
	fmt.Fprintf(mac, "%s\x00%s\x00%t", p.UID, p.Name, p.IsAdmin)
	return hex.EncodeToString(mac.Sum(nil))
}

func (g *Gateway) Authenticate(r *http.Request) (Principal, error) {
	p := Principal{Source: "fnos-gateway"}
	if trusted, _ := r.Context().Value(gatewayConnectionKey{}).(bool); !trusted {
		return p, fmt.Errorf("飞牛身份只允许通过应用 Unix Socket 传入")
	}
	p.UID = singleHeader(r, "X-Trim-Userid")
	p.Name = singleHeader(r, "X-Trim-Username")
	admin := singleHeader(r, "X-Trim-Isadmin")
	uid, err := strconv.ParseUint(p.UID, 10, 32)
	if err != nil || strconv.FormatUint(uid, 10) != p.UID || len(p.Name) == 0 || len(p.Name) > 64 || !utf8.ValidString(p.Name) || strings.TrimSpace(p.Name) != p.Name || strings.ContainsFunc(p.Name, unicode.IsControl) || (admin != "true" && admin != "false") {
		return p, fmt.Errorf("飞牛登录身份缺失或无效，请重新登录飞牛")
	}
	p.IsAdmin = admin == "true"
	p.CSRF = g.csrf(p)
	if !CheckOrigin(r) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return p, fmt.Errorf("跨站请求被拒绝")
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && subtle.ConstantTimeCompare([]byte(singleHeader(r, "X-CSRF-Token")), []byte(p.CSRF)) != 1 {
		return p, fmt.Errorf("请求验证失败，请刷新应用页面")
	}
	return p, nil
}

// RequireAdmin guards the entire application, including static files. The
// platform decides whether the NAS session is valid; NetLite authorizes admins.
func (g *Gateway) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		p, err := g.Authenticate(r)
		if err != nil {
			fail(w, http.StatusUnauthorized, err)
			return
		}
		if !p.IsAdmin {
			fail(w, http.StatusForbidden, fmt.Errorf("仅飞牛管理员可以使用 NetLite"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (g *Gateway) Handler(w http.ResponseWriter, r *http.Request) {
	p, err := g.Authenticate(r)
	if err != nil {
		fail(w, http.StatusUnauthorized, err)
		return
	}
	if !p.IsAdmin {
		fail(w, http.StatusForbidden, fmt.Errorf("仅飞牛管理员可以使用 NetLite"))
		return
	}
	switch {
	case r.URL.Path == "/api/auth/state" && r.Method == "GET":
		write(w, 200, map[string]any{"mode": "fnos-gateway", "initialized": true})
	case r.URL.Path == "/api/auth/me" && r.Method == "GET":
		write(w, 200, p)
	default:
		http.NotFound(w, r)
	}
}
