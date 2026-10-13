package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io"
	"log"
	"net"
	"net/http"
	"netlite/internal/auth"
	"netlite/internal/certificates"
	"netlite/internal/dnsprovider"
	"netlite/internal/dnstask"
	"netlite/internal/model"
	"netlite/internal/panel"
	"netlite/internal/proxy"
	"netlite/internal/store"
	frontend "netlite/web"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, e error) {
	respond(w, status, map[string]string{"error": e.Error()})
}
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "gateway-health":
			if err := gatewayHealth(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		case "init-admin", "reset-password", "reset-https", "info":
			if err := command(os.Args[1:]); err != nil {
				log.Fatal(err)
			}
			return
		}
	}
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func run() error {
	listen := flag.String("listen", "127.0.0.1:18780", "management address")
	data := flag.String("data", "data", "private persistent directory")
	web := flag.String("web", "", "development frontend directory; releases embed the frontend")
	authMode := flag.String("auth-mode", "local", "local, token, or fnos-gateway")
	gatewaySocket := flag.String("gateway-socket", "", "fnOS gateway Unix Socket (absolute path in TRIM_APPDEST)")
	gatewayMode := flag.String("gateway-socket-mode", "0600", "gateway Socket permissions: 0600 or 0660")
	gatewayGroup := flag.Int("gateway-socket-group", -1, "optional gateway Socket group ID (must be a group available to the package user)")
	binary := flag.String("caddy", "caddy", "independent Caddy executable")
	admin := flag.String("caddy-admin", "127.0.0.1:20190", "private Caddy admin")
	httpChallenge := flag.String("acme-http-listen", "0.0.0.0:18082", "HTTP-01 validation listener (public TCP 80 must reach it)")
	proxyKind := flag.String("proxy-engine", "caddy", "proxy engine; currently caddy")
	noCaddy := flag.Bool("no-caddy", false, "development only: CRUD without engine")
	flag.Parse()
	if *authMode != "local" && *authMode != "token" && *authMode != "fnos-gateway" {
		return fmt.Errorf("authentication mode unavailable: %s", *authMode)
	}
	isGateway := *authMode == "fnos-gateway"
	if isGateway && (runtime.GOOS != "linux" || *gatewaySocket == "") {
		return fmt.Errorf("fnos-gateway requires Linux and --gateway-socket")
	}
	if !isGateway && *gatewaySocket != "" {
		return fmt.Errorf("--gateway-socket requires fnos-gateway authentication")
	}
	if *proxyKind != "caddy" {
		return fmt.Errorf("unsupported proxy engine; currently caddy is available")
	}
	_, challengePortText, err := net.SplitHostPort(*httpChallenge)
	if err != nil {
		return err
	}
	challengePort, err := strconv.Atoi(challengePortText)
	if err != nil || challengePort < 1024 || challengePort > 65535 {
		return fmt.Errorf("HTTP-01 must use an unprivileged port")
	}

	host, port, e := net.SplitHostPort(*admin)
	if e != nil || host != "127.0.0.1" {
		return fmt.Errorf("Caddy admin must be 127.0.0.1:port")
	}
	adminPort, e := strconv.Atoi(port)
	if e != nil {
		return e
	}
	_, mp, e := net.SplitHostPort(*listen)
	if e != nil {
		return e
	}
	managePort, e := strconv.Atoi(mp)
	if e != nil {
		return e
	}
	if managePort == adminPort || challengePort == managePort || challengePort == adminPort {
		return fmt.Errorf("management and admin ports must differ")
	}
	if isGateway {
		managePort = 0 // This mode has no management TCP listener.
	}
	abs, e := filepath.Abs(*data)
	if e != nil {
		return e
	}
	*data = abs
	if e = os.MkdirAll(*data, 0700); e != nil {
		return e
	}
	var token []byte
	if !isGateway {
		tokenPath := filepath.Join(*data, "admin-token")
		token, e = os.ReadFile(tokenPath)
		if os.IsNotExist(e) {
			b := make([]byte, 32)
			if _, e = rand.Read(b); e != nil {
				return e
			}
			token = []byte(hex.EncodeToString(b))
			e = os.WriteFile(tokenPath, token, 0600)
		}
		if e != nil {
			return e
		}
		token = []byte(strings.TrimSpace(string(token)))
		if len(token) < 32 {
			return fmt.Errorf("admin-token must have at least 32 characters")
		}
	}
	s, e := store.Open(filepath.Join(*data, "netlite.db"))
	if e != nil {
		return e
	}
	defer s.DB.Close()
	var localAuth *auth.Manager
	var gatewayAuth *auth.Gateway
	var identityProvider auth.Provider
	if isGateway {
		gatewayAuth, e = auth.NewGateway()
		identityProvider = gatewayAuth
	} else {
		localAuth, e = auth.Open(s.DB, string(token))
		identityProvider = localAuth
	}
	if e != nil {
		return e
	}
	engine := proxy.New(*binary, *data, *admin)
	var adapter proxy.ProxyEngine = engine
	var configMu sync.Mutex
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ddnsService, e := dnstask.OpenService(ctx, s, filepath.Join(*data, "dns"))
	if e != nil {
		return e
	}
	certService, e := certificates.Open(ctx, s, filepath.Join(*data, "certificates"), *httpChallenge)
	if e != nil {
		return e
	}
	certService.ResolveAccount = ddnsService.ResolveAccount
	engine.ResolveCertificate = certService.Files
	certService.Applied = adapter.Applied
	// Platform mode must not load or enforce a previous standalone TLS setting.
	panelService := &panel.Manager{DB: s, Certificates: certService, Port: managePort}
	if !isGateway {
		panelService, e = panel.Open(s, certService, managePort)
		if e != nil {
			return e
		}
	}
	defer panelService.Close()
	certService.Reload = func(check context.Context) error {
		panelErr := panelService.Reload(check)
		if *noCaddy {
			return panelErr
		}
		configMu.Lock()
		defer configMu.Unlock()
		return errors.Join(panelErr, adapter.ReloadCertificates(check))
	}
	ddnsService.AppliedAccountReference = func(id string) bool {
		if certService.AccountReferenced(id) {
			return true
		}
		applied, err := adapter.Applied()
		if err != nil {
			return true
		}
		for _, c := range applied.Certificates {
			if c.AccountID == id {
				return true
			}
		}
		return false
	}
	errCh := make(chan error, 2)
	engineDone := make(chan struct{})
	if !*noCaddy {
		go func() {
			defer close(engineDone)
			if e := adapter.Run(ctx); e != nil {
				errCh <- e
				cancel()
			}
		}()
	} else {
		close(engineDone)
	}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	if *authMode == "local" {
		r.Handle("/api/auth/*", http.HandlerFunc(localAuth.Handler))
	} else if isGateway {
		r.Handle("/api/auth/*", http.HandlerFunc(gatewayAuth.Handler))
	} else {
		r.Get("/api/auth/state", func(w http.ResponseWriter, r *http.Request) {
			respond(w, 200, map[string]any{"mode": "token", "initialized": true})
		})
	}
	r.Route("/api", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !auth.CheckOrigin(r) {
					fail(w, 403, fmt.Errorf("跨站请求被拒绝"))
					return
				}
				if *authMode != "token" {
					principal, err := identityProvider.Authenticate(r)
					if err != nil {
						fail(w, 401, err)
						return
					}
					if principal.MustChange {
						fail(w, 403, fmt.Errorf("请先修改临时密码"))
						return
					}
				} else {
					got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					if subtle.ConstantTimeCompare([]byte(got), token) != 1 {
						fail(w, 401, fmt.Errorf("invalid management token"))
						return
					}
				}
				next.ServeHTTP(w, r)
			})
		})
		panelHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if isGateway {
				fail(w, http.StatusForbidden, fmt.Errorf("面板登录和 HTTPS 由飞牛管理"))
				return
			}
			configMu.Lock()
			defer configMu.Unlock()
			panelService.Handler(w, req)
		})
		r.Handle("/panel", panelHandler)
		r.Handle("/panel/*", panelHandler)
		ddnsRouter := ddnsService.Router()
		r.Mount("/dns", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			configMu.Lock()
			defer configMu.Unlock()
			ddnsRouter.ServeHTTP(w, req)
		}))
		r.Mount("/certificates", certService.Router())
		r.Get("/dns-capabilities", func(w http.ResponseWriter, r *http.Request) {
			respond(w, 200, map[string]any{"certificate_providers": dnsprovider.Providers})
		})
		r.Get("/config", func(w http.ResponseWriter, r *http.Request) {
			c, e := s.Load()
			if e != nil {
				fail(w, 500, e)
				return
			}
			respond(w, 200, c)
		})
		r.Put("/config", func(w http.ResponseWriter, r *http.Request) {
			configMu.Lock()
			defer configMu.Unlock()
			var c model.Config
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
			dec.DisallowUnknownFields()
			if e := dec.Decode(&c); e != nil {
				fail(w, 400, e)
				return
			}
			if e := dec.Decode(new(any)); e != io.EOF {
				fail(w, 400, fmt.Errorf("expected one JSON object"))
				return
			}
			if e := c.Validate(managePort, adminPort, challengePort); e != nil {
				fail(w, 400, e)
				return
			}
			if e := panelService.ValidateConfig(c); e != nil {
				fail(w, 400, e)
				return
			}
			if e := ddnsService.ValidateCertificates(c); e != nil {
				fail(w, 400, e)
				return
			}
			c, e := s.Save(c)
			if e != nil {
				status := 500
				if errors.Is(e, store.ErrConflict) {
					status = 409
				}
				fail(w, status, e)
				return
			}
			for _, policy := range c.Certificates {
				if err := certService.Request(policy, false); err != nil {
					log.Printf("certificate %s scheduling deferred", policy.ID)
				}
			}
			respond(w, 200, c)
		})
		r.Get("/proxy/config", func(w http.ResponseWriter, r *http.Request) {
			configMu.Lock()
			defer configMu.Unlock()
			cfg, err := s.Load()
			if err != nil {
				fail(w, 500, err)
				return
			}
			view, err := adapter.Configuration(cfg)
			if err != nil {
				fail(w, 500, fmt.Errorf("cannot read Caddy configuration"))
				return
			}
			respond(w, 200, view)
		})
		r.Post("/proxy/validate", func(w http.ResponseWriter, r *http.Request) {
			configMu.Lock()
			defer configMu.Unlock()
			c, e := s.Load()
			if e == nil {
				e = certService.RequireReady(c)
			}
			if e == nil {
				check, stop := context.WithTimeout(r.Context(), 30*time.Second)
				defer stop()
				e = adapter.Validate(check, c)
			}
			if e != nil {
				fail(w, 422, e)
				return
			}
			respond(w, 200, map[string]bool{"valid": true})
		})
		r.Post("/proxy/apply", func(w http.ResponseWriter, r *http.Request) {
			if *noCaddy {
				fail(w, 503, fmt.Errorf("proxy disabled in no-caddy development mode"))
				return
			}
			configMu.Lock()
			defer configMu.Unlock()
			c, e := s.Load()
			if e == nil {
				e = certService.RequireReady(c)
			}
			if e == nil {
				check, stop := context.WithTimeout(r.Context(), 30*time.Second)
				defer stop()
				e = adapter.Apply(check, c)
			}
			if e != nil {
				fail(w, 422, e)
				return
			}
			respond(w, 200, map[string]any{"applied_revision": c.Revision})
		})
		r.Get("/status", func(w http.ResponseWriter, r *http.Request) {
			p, e := adapter.Status(r.Context())
			if e != nil {
				fail(w, 500, e)
				return
			}
			respond(w, 200, map[string]any{"proxy": p, "certificate_note": "certificates are issued and renewed by NetLite; inspect /api/certificates"})
		})

	})
	static := frontend.Handler()
	if *web != "" {
		static = http.FileServer(http.Dir(*web))
	}
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		static.ServeHTTP(w, r)
	}))
	var listener net.Listener
	var handler http.Handler = panelService.Wrap(r)
	if isGateway {
		mode, err := strconv.ParseUint(*gatewayMode, 8, 32)
		if err != nil {
			e = fmt.Errorf("invalid gateway Socket permissions")
		} else {
			listener, e = listenGateway(*gatewaySocket, os.FileMode(mode), *gatewayGroup)
		}
		// Health is local Socket-only, has no credentials or application data,
		// and is intentionally outside the public /app/netlite route.
		app := gatewayAuth.RequireAdmin(gatewayRoutes(r))
		handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/healthz" && req.Method == "GET" {
				respond(w, 200, map[string]string{"status": "ok"})
				return
			}
			app.ServeHTTP(w, req)
		})
	} else {
		listener, e = net.Listen("tcp", *listen)
	}
	if e != nil {
		cancel()
		<-engineDone
		return e
	}
	defer listener.Close()
	if !isGateway {
		listener = panelService.Listener(listener)
	}
	srv := &http.Server{Handler: handler, ConnContext: auth.GatewayConnContext, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if e := srv.Serve(listener); e != nil && e != http.ErrServerClosed {
			errCh <- e
			cancel()
		}
	}()
	certDone := make(chan struct{})
	go func() { defer close(certDone); certService.Run(ctx) }()
	ddnsDone := make(chan struct{})
	go func() { defer close(ddnsDone); ddnsService.Run(ctx) }()
	if *authMode == "local" {
		if ready, _ := localAuth.Initialized(); !ready {
			log.Printf("管理员尚未初始化，请执行 netlite info --data %s 获取初始化码", *data)
		}
	}
	address := *listen
	if isGateway {
		address = *gatewaySocket
	}
	log.Printf("NetLite listening on %s; authentication mode: %s; private data: %s", address, *authMode, *data)
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	srv.Shutdown(shutdown)
	<-engineDone
	<-ddnsDone
	<-certDone
	select {
	case e := <-errCh:
		return e
	default:
		return nil
	}
}
