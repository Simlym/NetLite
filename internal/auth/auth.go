// Package auth keeps local authentication separate from platform identity providers.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const CookieName = "netlite-session"

type Principal struct {
	Name       string `json:"username"`
	MustChange bool   `json:"must_change_password"`
	CSRF       string `json:"csrf_token,omitempty"`
	password   []byte
}

// Provider is the integration seam for fnOS. No gateway headers are trusted by
// the local provider; a future gateway provider must use a private listener.
type Provider interface {
	Authenticate(*http.Request) (Principal, error)
}

type Manager struct {
	DB        *sql.DB
	Bootstrap string
	mu        sync.Mutex
	attempts  map[string]attempt
}
type attempt struct {
	count int
	until time.Time
}

func Open(db *sql.DB, bootstrap string) (*Manager, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS admin_account (id INTEGER PRIMARY KEY CHECK(id=1), name TEXT NOT NULL, password BLOB NOT NULL, must_change INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS auth_session (id TEXT PRIMARY KEY, csrf TEXT NOT NULL, expires INTEGER NOT NULL, idle_until INTEGER NOT NULL, lifetime INTEGER NOT NULL);`)
	return &Manager{DB: db, Bootstrap: bootstrap, attempts: map[string]attempt{}}, err
}
func Random() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (m *Manager) Initialized() (bool, error) {
	var n int
	err := m.DB.QueryRow("SELECT COUNT(*) FROM admin_account").Scan(&n)
	return n != 0, err
}
func validate(name, password string) error {
	if len(name) < 1 || len(name) > 64 || strings.TrimSpace(name) != name {
		return fmt.Errorf("用户名须为 1–64 个字符，首尾不能有空格")
	}
	if len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("密码须为 12–72 字节，可使用长口令")
	}
	return nil
}
func (m *Manager) Create(name, password string, temporary bool) error {
	if err := validate(name, password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = m.DB.Exec("INSERT INTO admin_account(id,name,password,must_change) VALUES(1,?,?,?)", name, hash, temporary)
	return err
}
func (m *Manager) Reset(password string, temporary bool) error {
	if err := validate("admin", password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	tx, err := m.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE admin_account SET password=?,must_change=? WHERE id=1", hash, temporary)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("管理员尚未初始化")
	}
	if _, err = tx.Exec("DELETE FROM auth_session"); err != nil {
		return err
	}
	return tx.Commit()
}
func (m *Manager) Check(name, password string) (Principal, error) {
	var p Principal
	var hash []byte
	err := m.DB.QueryRow("SELECT name,password,must_change FROM admin_account WHERE id=1").Scan(&p.Name, &hash, &p.MustChange)
	if err != nil || p.Name != name || bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return p, fmt.Errorf("用户名或密码错误")
	}
	p.password = hash
	return p, nil
}
func (m *Manager) Authenticate(r *http.Request) (Principal, error) {
	var p Principal
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return p, fmt.Errorf("请登录")
	}
	now := time.Now().Unix()
	err = m.DB.QueryRow(`SELECT a.name,a.must_change,s.csrf FROM auth_session s JOIN admin_account a ON a.id=1 WHERE s.id=? AND s.expires>? AND s.idle_until>?`, digest(cookie.Value), now, now).Scan(&p.Name, &p.MustChange, &p.CSRF)
	if err != nil {
		return p, fmt.Errorf("登录已过期，请重新登录")
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(p.CSRF)) != 1 {
		return p, fmt.Errorf("请求验证失败，请刷新页面")
	}
	_, err = m.DB.Exec("UPDATE auth_session SET idle_until=MIN(expires,?+lifetime) WHERE id=?", now, digest(cookie.Value))
	return p, err
}
func (m *Manager) session(w http.ResponseWriter, r *http.Request, remember bool, password []byte) error {
	id, err := Random()
	if err != nil {
		return err
	}
	csrf, err := Random()
	if err != nil {
		return err
	}
	idle := int64(8 * 3600)
	age := 0
	if remember {
		idle = 7 * 24 * 3600
		age = 30 * 24 * 3600
	}
	now := time.Now().Unix()
	_, err = m.DB.Exec("DELETE FROM auth_session WHERE expires<=? OR idle_until<=?", now, now)
	if err != nil {
		return err
	}
	// A password reset racing a login must not issue a fresh session for the old password.
	res, err := m.DB.Exec("INSERT INTO auth_session SELECT ?,?,?,?,? FROM admin_account WHERE id=1 AND password=?", digest(id), csrf, now+30*24*3600, now+idle, idle, password)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("密码已修改，请重新登录")
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: id, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: age})
	return nil
}
func (m *Manager) allowed(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for key, a := range m.attempts {
		if now.After(a.until) {
			delete(m.attempts, key)
		}
	}
	a := m.attempts[host]
	if a.count >= 10 {
		return false
	}
	if len(m.attempts) >= 4096 && a.count == 0 {
		return false
	}
	a.count++
	if a.until.IsZero() {
		a.until = now.Add(10 * time.Minute)
	}
	m.attempts[host] = a
	return true
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, err error) {
	write(w, status, map[string]string{"error": err.Error()})
}
func read(w http.ResponseWriter, r *http.Request, value any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("需要 JSON 请求")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	return dec.Decode(value)
}

// CheckOrigin also protects unauthenticated login and bootstrap requests.
func CheckOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host && (u.Scheme == "http" || u.Scheme == "https")
}
func (m *Manager) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && !CheckOrigin(r) {
		fail(w, 403, fmt.Errorf("跨站请求被拒绝"))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/")
	if path == "state" && r.Method == "GET" {
		ready, err := m.Initialized()
		if err != nil {
			fail(w, 500, fmt.Errorf("无法读取认证状态"))
			return
		}
		write(w, 200, map[string]any{"mode": "local", "initialized": ready})
		return
	}
	if (path == "setup" || path == "login") && r.Method == "POST" {
		if !m.allowed(r) {
			fail(w, 429, fmt.Errorf("尝试过多，请 10 分钟后再试"))
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Code     string `json:"code"`
			Remember bool   `json:"remember"`
		}
		if err := read(w, r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		if path == "setup" {
			ready, err := m.Initialized()
			if err != nil {
				fail(w, 500, fmt.Errorf("无法读取认证状态"))
				return
			}
			if ready {
				fail(w, 409, fmt.Errorf("管理员已初始化"))
				return
			}
			if subtle.ConstantTimeCompare([]byte(req.Code), []byte(m.Bootstrap)) != 1 {
				fail(w, 401, fmt.Errorf("初始化码错误"))
				return
			}
			if err := m.Create(req.Username, req.Password, false); err != nil {
				fail(w, 400, fmt.Errorf("无法初始化管理员：%v", err))
				return
			}
		}
		principal, err := m.Check(req.Username, req.Password)
		if err != nil {
			fail(w, 401, err)
			return
		}
		if err := m.session(w, r, req.Remember, principal.password); err != nil {
			fail(w, 500, fmt.Errorf("无法创建会话"))
			return
		}
		write(w, 200, map[string]bool{"ok": true})
		return
	}
	p, err := m.Authenticate(r)
	if err != nil {
		fail(w, 401, err)
		return
	}
	switch {
	case path == "me" && r.Method == "GET":
		write(w, 200, p)
	case path == "logout" && r.Method == "POST":
		cookie, _ := r.Cookie(CookieName)
		if _, err := m.DB.Exec("DELETE FROM auth_session WHERE id=?", digest(cookie.Value)); err != nil {
			fail(w, 500, fmt.Errorf("无法注销会话"))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: CookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode})
		write(w, 200, map[string]bool{"ok": true})
	case path == "password" && r.Method == "POST":
		if !m.allowed(r) {
			fail(w, 429, fmt.Errorf("尝试过多，请稍后再试"))
			return
		}
		var req struct {
			Old string `json:"old_password"`
			New string `json:"new_password"`
		}
		if err := read(w, r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		if _, err := m.Check(p.Name, req.Old); err != nil {
			fail(w, 401, err)
			return
		}
		if req.Old == req.New {
			fail(w, 400, fmt.Errorf("新密码不能与原密码相同"))
			return
		}
		if err := m.Reset(req.New, false); err != nil {
			fail(w, 400, err)
			return
		}
		principal, err := m.Check(p.Name, req.New)
		if err != nil {
			fail(w, 401, err)
			return
		}
		if err := m.session(w, r, false, principal.password); err != nil {
			fail(w, 500, fmt.Errorf("请使用新密码重新登录"))
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}
