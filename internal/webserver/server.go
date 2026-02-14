package webserver

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/store"
)

//go:embed static
var staticFS embed.FS

// Options configures web server behavior.
type Options struct {
	Host      string
	Port      int
	TLSMode   string
	CertFile  string
	KeyFile   string
	AuthToken string
	RateLimit float64
}

// Server hosts the HTTP API and WebSocket session stream bridge.
type Server struct {
	store      *store.Store
	httpServer *http.Server
	port       int
	host       string
	tlsMode    string
	certFile   string
	keyFile    string
	authToken  string
	rateLimit  float64
}

// New constructs a web server bound to host:port.
func New(s *store.Store, opts Options) *Server {
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		host = "127.0.0.1"
	}

	port := opts.Port
	if port <= 0 {
		port = 8080
	}

	srv := &Server{
		store:     s,
		host:      host,
		port:      port,
		tlsMode:   strings.TrimSpace(opts.TLSMode),
		certFile:  strings.TrimSpace(opts.CertFile),
		keyFile:   strings.TrimSpace(opts.KeyFile),
		authToken: strings.TrimSpace(opts.AuthToken),
		rateLimit: opts.RateLimit,
	}

	mux := http.NewServeMux()
	srv.setupRoutes(mux)

	handler := corsMiddleware(logMiddleware(rateLimitMiddleware(srv.rateLimit, authMiddleware(srv.authToken, mux))))
	srv.httpServer = &http.Server{
		Addr:              srv.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return srv
}

// Start starts the server in a background goroutine and returns immediately.
func (srv *Server) Start() error {
	if srv.httpServer == nil {
		return fmt.Errorf("webserver not initialized")
	}

	if srv.tlsMode != "" {
		var cert tls.Certificate
		var err error

		switch srv.tlsMode {
		case "self-signed":
			cert, err = generateSelfSignedCert(srv.host)
			if err != nil {
				return fmt.Errorf("generating self-signed certificate: %w", err)
			}
		case "custom":
			cert, err = tls.LoadX509KeyPair(srv.certFile, srv.keyFile)
			if err != nil {
				return fmt.Errorf("loading TLS certificate: %w", err)
			}
		default:
			return fmt.Errorf("unsupported TLS mode: %q", srv.tlsMode)
		}

		srv.httpServer.TLSConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		}
	}

	ln, err := net.Listen("tcp", srv.Addr())
	if err != nil {
		return err
	}

	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		srv.port = tcpAddr.Port
		srv.httpServer.Addr = srv.Addr()
	}

	go func() {
		var err error
		if srv.tlsMode != "" {
			err = srv.httpServer.ServeTLS(ln, "", "")
		} else {
			err = srv.httpServer.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			debug.LogKV("webserver", "server stopped with error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully stops the HTTP server.
func (srv *Server) Shutdown(ctx context.Context) error {
	if srv.httpServer == nil {
		return nil
	}
	return srv.httpServer.Shutdown(ctx)
}

// Addr returns the bound host:port address.
func (srv *Server) Addr() string {
	return net.JoinHostPort(srv.host, strconv.Itoa(srv.port))
}

// Scheme returns the URL scheme for the running server.
func (srv *Server) Scheme() string {
	if srv.tlsMode != "" {
		return "https"
	}
	return "http"
}

func (srv *Server) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/project", srv.handleProject)

	mux.HandleFunc("GET /api/plans", srv.handlePlans)
	mux.HandleFunc("GET /api/plans/{id}", srv.handlePlanByID)
	// Plan write endpoints
	mux.HandleFunc("POST /api/plans", srv.handleCreatePlan)
	mux.HandleFunc("PUT /api/plans/{id}", srv.handleUpdatePlan)
	mux.HandleFunc("PUT /api/plans/{id}/phases/{phaseId}", srv.handleUpdatePlanPhase)
	mux.HandleFunc("POST /api/plans/{id}/activate", srv.handleActivatePlan)
	mux.HandleFunc("DELETE /api/plans/{id}", srv.handleDeletePlan)

	mux.HandleFunc("GET /api/issues", srv.handleIssues)
	mux.HandleFunc("GET /api/issues/{id}", srv.handleIssueByID)
	// Issue write endpoints
	mux.HandleFunc("POST /api/issues", srv.handleCreateIssue)
	mux.HandleFunc("PUT /api/issues/{id}", srv.handleUpdateIssue)

	// Doc endpoints (read + write)
	mux.HandleFunc("GET /api/docs", srv.handleDocs)
	mux.HandleFunc("GET /api/docs/{id}", srv.handleDocByID)
	mux.HandleFunc("POST /api/docs", srv.handleCreateDoc)
	mux.HandleFunc("PUT /api/docs/{id}", srv.handleUpdateDoc)

	mux.HandleFunc("GET /api/turns", srv.handleTurns)
	mux.HandleFunc("GET /api/turns/{id}", srv.handleTurnByID)
	mux.HandleFunc("GET /api/spawns", srv.handleSpawns)
	mux.HandleFunc("GET /api/spawns/{id}", srv.handleSpawnByID)
	mux.HandleFunc("GET /api/sessions", srv.handleSessions)
	mux.HandleFunc("GET /api/sessions/{id}", srv.handleSessionByID)
	mux.HandleFunc("GET /api/stats/loops", srv.handleLoopStats)
	mux.HandleFunc("GET /api/stats/profiles", srv.handleProfileStats)

	mux.HandleFunc("GET /ws/sessions/{id}", srv.handleSessionWebSocket)
	mux.HandleFunc("GET /ws/terminal", srv.handleTerminalWebSocket)

	mux.HandleFunc("GET /api/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	staticHandler := http.FileServer(http.FS(staticFS))
	mux.Handle("GET /static/", staticHandler)

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		data, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "failed to load index", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}
