package dnstask

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"netlite/internal/dnsprovider"
	"netlite/internal/model"
	"strings"
	"sync"
	"time"
)

type RecordResult struct {
	Domain  string `json:"domain"`
	Type    string `json:"type"`
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

type State struct {
	Running          bool           `json:"running"`
	Message          string         `json:"message"`
	ResultKnown      bool           `json:"result_known"`
	Checking         bool           `json:"checking"`
	IPv4             string         `json:"ipv4"`
	IPv6             string         `json:"ipv6"`
	IPv4Result       string         `json:"ipv4_result"`
	IPv6Result       string         `json:"ipv6_result"`
	ProviderRequests int            `json:"provider_requests"`
	UpdateRequests   int            `json:"update_requests"`
	HTTPStatus       int            `json:"http_status"`
	NetworkError     bool           `json:"network_error"`
	LastCheck        time.Time      `json:"last_check"`
	LastFinished     time.Time      `json:"last_finished"`
	LastRequest      time.Time      `json:"last_request"`
	Records          []RecordResult `json:"records,omitempty"`
}

type TaskEngine interface {
	Start(context.Context, Task) error
	Stop(string)
	CheckNow(string) error
	Close()
	State(string) State
}

type job struct {
	mu     sync.Mutex
	state  State
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}
}

type Engine struct {
	mu         sync.Mutex
	jobs       map[string]*job
	history    map[string]State
	slots      chan struct{}
	client     *http.Client
	provider   func(model.DNSCredential) *dnsprovider.Client
	onFinished func(string, State) error
}

func New() *Engine {
	return &Engine{jobs: map[string]*job{}, history: map[string]State{}, slots: make(chan struct{}, 4),
		client:   &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		provider: dnsprovider.New}
}

func (e *Engine) Start(ctx context.Context, t Task) error {
	if err := Validate(t); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Own the slices so callers cannot mutate an active job.
	t.IPv4.Domains = append([]string(nil), t.IPv4.Domains...)
	t.IPv6.Domains = append([]string(nil), t.IPv6.Domains...)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stop(t.ID)
	ctx, cancel := context.WithCancel(ctx)
	j := &job{cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1), state: State{Running: true, Message: "等待检查"}}
	e.jobs[t.ID] = j
	go e.run(ctx, t, j)
	return nil
}

func (e *Engine) run(ctx context.Context, t Task, j *job) {
	defer close(j.done)
	defer func() { j.mu.Lock(); j.state.Running, j.state.Checking = false, false; j.mu.Unlock() }()
	for {
		select {
		case e.slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		if ctx.Err() != nil {
			<-e.slots
			return
		}
		j.mu.Lock()
		j.state = State{Running: true, Checking: true, LastCheck: time.Now().UTC(), Message: "正在检查解析记录"}
		j.mu.Unlock()
		check, cancel := context.WithTimeout(ctx, 5*time.Minute)
		s := e.check(check, t)
		cancel()
		<-e.slots
		if ctx.Err() != nil {
			return
		}
		if e.onFinished != nil {
			if err := e.onFinished(t.ID, s); err != nil {
				s.Message += "；检查记录保存失败"
			}
		}
		j.mu.Lock()
		j.state = s
		j.mu.Unlock()
		timer := time.NewTimer(time.Duration(t.Interval) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-j.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// One queued request per job; checks never overlap for the same task.
func (e *Engine) CheckNow(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	j := e.jobs[id]
	if j == nil {
		return fmt.Errorf("task is not running")
	}
	select {
	case j.wake <- struct{}{}:
	default:
	}
	return nil
}

func (e *Engine) stop(id string) {
	if j := e.jobs[id]; j != nil {
		j.cancel()
		<-j.done
		j.mu.Lock()
		e.history[id] = cloneState(j.state)
		j.mu.Unlock()
		delete(e.jobs, id)
	}
}

func (e *Engine) Stop(id string) { e.mu.Lock(); defer e.mu.Unlock(); e.stop(id) }
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.jobs {
		e.stop(id)
	}
}
func cloneState(s State) State { s.Records = append([]RecordResult(nil), s.Records...); return s }
func (e *Engine) State(id string) State {
	e.mu.Lock()
	defer e.mu.Unlock()
	if j := e.jobs[id]; j != nil {
		j.mu.Lock()
		defer j.mu.Unlock()
		return cloneState(j.state)
	}
	s := cloneState(e.history[id])
	s.Running, s.Checking = false, false
	if s.Message == "" {
		s.Message = "未运行"
	}
	return s
}

func (e *Engine) getIP(ctx context.Context, source Source, v4 bool) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", source.URL, nil)
	if err != nil {
		return "", fmt.Errorf("invalid IP source")
	}
	res, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("IP lookup failed")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 129))
	if err != nil || res.StatusCode != 200 || len(b) > 128 {
		return "", fmt.Errorf("IP lookup failed")
	}
	ip := net.ParseIP(strings.TrimSpace(string(b)))
	if ip == nil || (ip.To4() != nil) != v4 {
		return "", fmt.Errorf("invalid IP address family")
	}
	return ip.String(), nil
}

func (e *Engine) check(ctx context.Context, t Task) State {
	s := State{Running: true, Checking: true, LastCheck: time.Now().UTC(), IPv4Result: "disabled", IPv6Result: "disabled"}
	p := e.provider(model.DNSCredential{Provider: t.Provider, ID: t.CredentialID, Secret: t.Secret})
	for i, source := range []Source{t.IPv4, t.IPv6} {
		if len(source.Domains) == 0 {
			continue
		}
		typ := "A"
		if i == 1 {
			typ = "AAAA"
		}
		ip, err := e.getIP(ctx, source, i == 0)
		result := "unchanged"
		if err != nil {
			result = "ip_failed"
		} else if i == 0 {
			s.IPv4 = ip
		} else {
			s.IPv6 = ip
		}
		for _, domain := range source.Domains {
			r := RecordResult{Domain: domain, Type: typ, Result: "ip_failed"}
			if err == nil {
				changed, syncErr := p.Sync(ctx, strings.ToLower(domain), typ, ip, t.TTL)
				r.Result = "unchanged"
				if syncErr != nil {
					r.Result, r.Message = "failed", syncErr.Error()
					result = "failed"
				} else if changed {
					r.Result = "success"
					if result != "failed" {
						result = "success"
					}
				}
			}
			s.Records = append(s.Records, r)
		}
		if i == 0 {
			s.IPv4Result = result
		} else {
			s.IPv6Result = result
		}
	}
	stats := p.Stats()
	s.ProviderRequests, s.UpdateRequests, s.HTTPStatus, s.NetworkError = stats.ProviderRequests, stats.UpdateRequests, stats.HTTPStatus, stats.NetworkError
	s.LastRequest = stats.LastRequest
	s.Checking, s.ResultKnown = false, true
	s.LastFinished = time.Now().UTC()
	s.Message = "已检查解析记录，无需更新"
	if s.IPv4Result == "success" || s.IPv6Result == "success" {
		s.Message = "DNS 更新成功"
	}
	if s.IPv4Result == "failed" || s.IPv6Result == "failed" {
		s.Message = "部分解析记录同步失败，请查看详情"
	}
	if s.IPv4Result == "ip_failed" || s.IPv6Result == "ip_failed" {
		s.Message = "公网 IP 获取失败"
	}
	return s
}
