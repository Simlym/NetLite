package ddnsgo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type State struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
	// Process readiness is distinct from DNS update success.
	ResultKnown      bool      `json:"result_known"`
	Checking         bool      `json:"checking"`
	IPv4             string    `json:"ipv4"`
	IPv6             string    `json:"ipv6"`
	IPv4Result       string    `json:"ipv4_result"`
	IPv6Result       string    `json:"ipv6_result"`
	ProviderRequests int       `json:"provider_requests"`
	UpdateRequests   int       `json:"update_requests"`
	HTTPStatus       int       `json:"http_status"`
	NetworkError     bool      `json:"network_error"`
	LastCheck        time.Time `json:"last_check"`
	LastFinished     time.Time `json:"last_finished"`
	LastRequest      time.Time `json:"last_request"`
}

// ProcessEngine is the contract consumed by the upcoming task manager.
type ProcessEngine interface {
	Start(context.Context, Task) error
	Stop(string)
	Close()
	State(string) State
}
type child struct {
	onFinished func(State) error
	mu         sync.Mutex
	state      State
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	done       chan struct{}
}
type Engine struct {
	onFinished  func(string, State) error
	binary, dir string
	mu          sync.Mutex
	history     map[string]State
	children    map[string]*child
	command     func(context.Context, string, ...string) *exec.Cmd
}

func New(binary, dir string) *Engine {
	return &Engine{binary: binary, dir: dir, children: map[string]*child{}, history: map[string]State{}, command: exec.CommandContext}
}

// Start replaces only this engine's child for the given task. One task per process
// preserves independent intervals and associates process state with a stable task ID.
func (e *Engine) Start(ctx context.Context, t Task) error {
	data, err := Render(t)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stop(t.ID)
	dir := filepath.Join(e.dir, t.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create private DDNS directory")
	}
	if err = protectDirectory(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	// This directory is application-owned; generated files never enter Git or APIs.
	if err = os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("cannot write private DDNS configuration")
	}
	if err = os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("cannot protect DDNS configuration")
	}
	childCtx, cancel := context.WithCancel(ctx)
	cmd := e.command(childCtx, e.binary, "-noweb", "-c", path, "-f", strconv.Itoa(t.Interval))
	configureProcess(cmd)
	c := &child{cmd: cmd, cancel: cancel, done: make(chan struct{}), state: State{Message: "等待网络或首次检查"}}
	if e.onFinished != nil {
		c.onFinished = func(s State) error { return e.onFinished(t.ID, s) }
	}
	// Accept only schema-versioned bridge events; discard all ordinary output.
	cmd.Stdout = &eventWriter{child: c}
	cmd.Stderr = nil
	if err = cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("cannot start ddns-go; check executable path and permissions")
	}
	e.children[t.ID] = c
	go func() { cmd.Wait(); close(c.done) }()
	return nil
}
func (e *Engine) stop(id string) {
	if c := e.children[id]; c != nil {
		c.cancel()
		<-c.done
		c.mu.Lock()
		s := c.state
		s.Checking = false
		s.Running = false
		e.history[id] = s
		c.mu.Unlock()
		delete(e.children, id)
	}
	if taskID.MatchString(id) {
		// Generated plaintext credentials are needed only while the child runs.
		_ = os.Remove(filepath.Join(e.dir, id, "config.json"))
	}
}
func (e *Engine) Stop(id string) { e.mu.Lock(); defer e.mu.Unlock(); e.stop(id) }
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.children {
		e.stop(id)
	}
}
func (e *Engine) State(id string) State {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.children[id]
	if c == nil {
		s := e.history[id]
		s.Running = false
		if s.Message == "" {
			s.Message = "not running"
		}
		return s
	}
	c.mu.Lock()
	s := c.state
	c.mu.Unlock()
	select {
	case <-c.done:
		s.Running = false
		s.Checking = false
		s.Message = "ddns-go 已退出；请检查引擎和网络"
	default:
		s.Running = true
	}
	return s
}
