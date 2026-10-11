package dnstask

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"log"
	"net/http"
	"netlite/internal/dnsaccount"
	"netlite/internal/model"
	"netlite/internal/store"
	"strings"
	"sync"
	"time"
)

type Service struct {
	store                   *store.Store
	engine                  TaskEngine
	vault                   *dnsaccount.Vault
	AppliedAccountReference func(string) bool
	ctx                     context.Context
	mu                      sync.Mutex
	active                  map[string][32]byte
	problems                map[string]string
}
type accountInput struct {
	model.DNSAccount
	Secret string `json:"secret"`
}
type taskView struct {
	model.DDNSTask
	State State `json:"state"`
}

func OpenService(ctx context.Context, s *store.Store, dir string) (*Service, error) {
	vault, e := dnsaccount.Open(s, dir)
	if e != nil {
		return nil, e
	}
	if _, e = s.DB.Exec(`CREATE TABLE IF NOT EXISTS dns_tasks(id TEXT PRIMARY KEY,body TEXT NOT NULL);`); e != nil {
		return nil, e
	}
	if _, e = s.DB.Exec(`CREATE TABLE IF NOT EXISTS dns_history(seq INTEGER PRIMARY KEY AUTOINCREMENT,task_id TEXT NOT NULL,body TEXT NOT NULL); CREATE INDEX IF NOT EXISTS dns_history_task ON dns_history(task_id,seq);`); e != nil {
		return nil, e
	}
	svc := &Service{store: s, engine: New(), vault: vault, ctx: ctx, active: map[string][32]byte{}, problems: map[string]string{}}
	svc.engine.(*Engine).onFinished = svc.saveResult
	return svc, nil
}
func (s *Service) accounts() ([]model.DNSAccount, error)                 { return s.vault.List() }
func (s *Service) ResolveAccount(id string) (model.DNSCredential, error) { return s.vault.Resolve(id) }
func (s *Service) accountUsed(id string) (bool, error) {
	tasks, err := s.tasks()
	if err != nil { return false, err }
	for _, task := range tasks { if task.AccountID == id { return true, nil } }
	cfg, e := s.store.Load()
	if e != nil {
		return false, e
	}
	for _, c := range cfg.Certificates {
		if c.AccountID == id {
			return true, nil
		}
	}
	return s.AppliedAccountReference != nil && s.AppliedAccountReference(id), nil
}
func (s *Service) ValidateCertificates(cfg model.Config) error {
	for _, c := range cfg.Certificates {
		if c.AccountID != "" {
			if _, e := s.vault.Resolve(c.AccountID); e != nil {
				return fmt.Errorf("证书 %s 引用的 DNS 账号不存在或不可用", c.ID)
			}
		}
	}
	return nil
}
func (s *Service) tasks() ([]model.DDNSTask, error) {
	rows, e := s.store.DB.Query("SELECT body FROM dns_tasks ORDER BY id")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.DDNSTask{}
	for rows.Next() {
		var b string
		var t model.DDNSTask
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(b), &t); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Service) renderTask(t model.DDNSTask) (Task, error) {
	a, e := s.vault.Resolve(t.AccountID)
	if e != nil {
		return Task{}, e
	}
	return Task{ID: t.ID, Name: t.Name, Provider: a.Provider, CredentialID: a.ID, Secret: a.Secret, IPv4: Source{t.IPv4Domains, t.IPv4URL}, IPv6: Source{t.IPv6Domains, t.IPv6URL}, Interval: t.Interval, TTL: t.TTL}, nil
}
func (s *Service) conflict(t model.DDNSTask) error {
	if !t.Enabled {
		return nil
	}
	all, e := s.tasks()
	if e != nil {
		return e
	}
	domains := map[string]bool{}
	for _, d := range t.IPv4Domains {
		domains["A:"+d] = true
	}
	for _, d := range t.IPv6Domains {
		domains["AAAA:"+d] = true
	}
	for _, other := range all {
		if other.ID == t.ID || !other.Enabled {
			continue
		}
		for _, d := range other.IPv4Domains {
			if domains["A:"+d] {
				return fmt.Errorf("该 A 记录已由其他任务管理")
			}
		}
		for _, d := range other.IPv6Domains {
			if domains["AAAA:"+d] {
				return fmt.Errorf("该 AAAA 记录已由其他任务管理")
			}
		}
	}
	return nil
}

func (s *Service) reconcile() error {
	tasks, e := s.tasks()
	if e != nil {
		return e
	}
	keep := map[string]bool{}
	for _, t := range tasks {
		if !t.Enabled {
			continue
		}
		keep[t.ID] = true
		task, err := s.renderTask(t)
		if err == nil {
			err = s.conflict(t)
		}
		var b []byte
		if err == nil {
			err = Validate(task)
			if err == nil { b, err = json.Marshal(task) }
		}
		if err != nil {
			s.engine.Stop(t.ID)
			delete(s.active, t.ID)
			s.problems[t.ID] = err.Error()
			continue
		}
		hash := sha256.Sum256(append(b, []byte(fmt.Sprint(t.Interval))...))
		if s.active[t.ID] == hash && s.engine.State(t.ID).Running {
			continue
		}
		if err = s.engine.Start(s.ctx, task); err != nil {
			s.problems[t.ID] = err.Error()
			delete(s.active, t.ID)
		} else {
			s.active[t.ID] = hash
			delete(s.problems, t.ID)
		}
	}
	for id := range s.active {
		if !keep[id] {
			s.engine.Stop(id)
			delete(s.active, id)
		}
	}
	for id := range s.problems {
		if !keep[id] {
			delete(s.problems, id)
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	defer s.engine.Close()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		if err := s.reconcile(); err != nil { log.Printf("DDNS task reconciliation failed: %v", err) }
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.mu.Lock(); defer s.mu.Unlock(); next.ServeHTTP(w, r) })
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		accounts, e := s.accounts()
		if e != nil {
			serviceError(w, 500, "无法读取账号")
			return
		}
		tasks, e := s.tasks()
		if e != nil {
			serviceError(w, 500, "无法读取任务")
			return
		}
		views := []taskView{}
		for _, t := range tasks {
			state := s.engine.State(t.ID)
			if state.LastFinished.IsZero() {
				history, err := s.history(t.ID, 1)
				if err != nil {
					serviceError(w, 500, "无法读取检查记录")
					return
				}
				if len(history) > 0 {
					previous := history[0]
					previous.Running, previous.Checking = state.Running, state.Checking
					if state.Checking {
						previous.Message = state.Message
					}
					state = previous
				}
			}
			if !t.Enabled {
				state.Running = false
				state.Checking = false
				state.Message = "已停用"
			} else if p := s.problems[t.ID]; p != "" {
				state.Message = p
			}
			views = append(views, taskView{t, state})
		}
		serviceJSON(w, 200, map[string]any{"accounts": accounts, "tasks": views, "available": true, "engine": "native", "version": "1"})
	})
	r.Get("/accounts", func(w http.ResponseWriter, r *http.Request) {
		a, e := s.accounts()
		if e != nil {
			serviceError(w, 500, "无法读取 DNS 账号")
			return
		}
		serviceJSON(w, 200, map[string]any{"accounts": a})
	})
	r.Put("/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var a accountInput
		if !serviceDecode(w, r, &a) {
			return
		}
		id := chi.URLParam(r, "id")
		a.ID = id
		a.Name = strings.TrimSpace(a.Name)
		if !taskID.MatchString(id) || a.Name == "" || len(a.Name) > 500 {
			serviceError(w, 400, "请填写有效账号名称")
			return
		}
		switch a.Provider {
		case "cloudflare", "dnspod", "tencentcloud", "alidns":
		default:
			serviceError(w, 400, "不支持的服务商")
			return
		}
		if a.Provider != "cloudflare" && a.CredentialID == "" {
			serviceError(w, 400, "请填写账号 ID / SecretId")
			return
		}
		var sealed []byte
		var oldBody string
		e := s.store.DB.QueryRow("SELECT body,secret FROM dns_accounts WHERE id=?", id).Scan(&oldBody, &sealed)
		if e != nil && e != sql.ErrNoRows {
			serviceError(w, 500, "无法读取账号")
			return
		}
		if a.Secret != "" {
			var previous model.DNSAccount
			_ = json.Unmarshal([]byte(oldBody), &previous)
			if previous.Provider != "" && previous.Provider != a.Provider {
				used, err := s.accountUsed(id)
				if err != nil {
					serviceError(w, 500, "无法检查账号引用")
					return
				}
				if used {
					serviceError(w, 409, "此账号已被证书引用，不能更换服务商；请新建账号")
					return
				}
			}
			sealed, e = s.vault.Encrypt(id, a.Secret)
			if e != nil {
				serviceError(w, 500, "无法保存凭据")
				return
			}
		} else {
			var old model.DNSAccount
			_ = json.Unmarshal([]byte(oldBody), &old)
			if e == sql.ErrNoRows || old.Provider != a.Provider {
				serviceError(w, 400, "新账号或更换服务商时必须填写密钥")
				return
			}
		}
		a.HasSecret = true
		b, _ := json.Marshal(a.DNSAccount)
		_, e = s.store.DB.Exec("INSERT INTO dns_accounts VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body,secret=excluded.secret", id, string(b), sealed)
		if e != nil {
			serviceError(w, 500, "无法保存账号")
			return
		}
		if err := s.reconcile(); err != nil { log.Printf("DDNS task reconciliation failed: %v", err) }
		serviceJSON(w, 200, a.DNSAccount)
	})
	r.Delete("/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tasks, e := s.tasks()
		if e != nil {
			serviceError(w, 500, "无法读取任务")
			return
		}
		for _, t := range tasks {
			if t.AccountID == id {
				serviceError(w, 409, "请先删除使用此账号的任务")
				return
			}
		}
		used, err := s.accountUsed(id)
		if err != nil {
			serviceError(w, 500, "无法检查证书引用")
			return
		}
		if used {
			serviceError(w, 409, "此账号仍被证书或已应用配置引用，请移除引用并应用后再删除")
			return
		}
		if _, e = s.store.DB.Exec("DELETE FROM dns_accounts WHERE id=?", id); e != nil {
			serviceError(w, 500, "无法删除账号")
			return
		}
		serviceJSON(w, 200, map[string]bool{"deleted": true})
	})
	r.Put("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		var t model.DDNSTask
		if !serviceDecode(w, r, &t) {
			return
		}
		t.ID = chi.URLParam(r, "id")
		t.Name = strings.TrimSpace(t.Name)
		for i := range t.IPv4Domains { t.IPv4Domains[i] = strings.ToLower(t.IPv4Domains[i]) }
		for i := range t.IPv6Domains { t.IPv6Domains[i] = strings.ToLower(t.IPv6Domains[i]) }
		if len(t.Name) > 500 {
			serviceError(w, 400, "任务名称、域名数量或 TTL 不正确")
			return
		}
		if t.Name == "" {
			serviceError(w, 400, "请填写任务名称")
			return
		}
		task, e := s.renderTask(t)
		if e == nil {
			e = Validate(task)
		}
		if e == nil {
			e = s.conflict(t)
		}
		if e != nil {
			serviceError(w, 400, e.Error())
			return
		}
		b, _ := json.Marshal(t)
		if _, e = s.store.DB.Exec("INSERT INTO dns_tasks VALUES(?,?) ON CONFLICT(id) DO UPDATE SET body=excluded.body", t.ID, string(b)); e != nil {
			serviceError(w, 500, "无法保存任务")
			return
		}
		if err := s.reconcile(); err != nil { log.Printf("DDNS task reconciliation failed: %v", err) }
		serviceJSON(w, 200, t)
	})
	r.Delete("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if _, e := s.store.DB.Exec("DELETE FROM dns_tasks WHERE id=?", id); e != nil {
			serviceError(w, 500, "无法删除任务")
			return
		}
		s.engine.Stop(id)
		if _, e := s.store.DB.Exec("DELETE FROM dns_history WHERE task_id=?", id); e != nil {
			serviceError(w, 500, "无法删除检查记录")
			return
		}
		delete(s.active, id)
		delete(s.problems, id)
		serviceJSON(w, 200, map[string]bool{"deleted": true})
	})
	r.Get("/tasks/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var exists int
		if err := s.store.DB.QueryRow("SELECT 1 FROM dns_tasks WHERE id=?", id).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				serviceError(w, 404, "任务不存在")
			} else {
				serviceError(w, 500, "无法读取任务")
			}
			return
		}
		history, err := s.history(id, 100)
		if err != nil {
			serviceError(w, 500, "无法读取检查记录")
			return
		}
		serviceJSON(w, 200, map[string]any{"history": history})
	})
	r.Post("/tasks/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tasks, e := s.tasks()
		if e != nil {
			serviceError(w, 500, "无法读取任务")
			return
		}
		for _, t := range tasks {
			if t.ID == id {
				if !t.Enabled {
					serviceError(w, 400, "请先启用任务")
					return
				}
				if err := s.reconcile(); err != nil { serviceError(w, 500, "无法读取任务配置"); return }
				if p := s.problems[id]; p != "" {
					serviceError(w, 400, p)
					return
				}
				if err := s.engine.CheckNow(id); err != nil { serviceError(w, 503, "任务未运行"); return }
				serviceJSON(w, 202, map[string]string{"message": "已请求重新检查，DNS 同步结果尚未确认"})
				return
			}
		}
		serviceError(w, 404, "任务不存在")
	})
	return r
}
func serviceJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func serviceError(w http.ResponseWriter, status int, message string) {
	serviceJSON(w, status, map[string]string{"error": message})
}
func serviceDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		serviceError(w, 400, "配置格式不正确")
		return false
	}
	if e := d.Decode(new(any)); e != io.EOF {
		serviceError(w, 400, "只能提交一个配置对象")
		return false
	}
	return true
}

