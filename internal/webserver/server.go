package webserver

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/store"
)

//go:embed static
var staticFS embed.FS

// Server hosts the HTTP API and WebSocket session stream bridge.
type Server struct {
	store      *store.Store
	httpServer *http.Server
	port       int
	host       string
}

// New constructs a web server bound to host:port.
func New(s *store.Store, host string, port int) *Server {
	if host == "" {
		host = "127.0.0.1"
	}

	srv := &Server{
		store: s,
		host:  host,
		port:  port,
	}

	mux := http.NewServeMux()
	srv.setupRoutes(mux)

	handler := corsMiddleware(logMiddleware(mux))
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

	ln, err := net.Listen("tcp", srv.Addr())
	if err != nil {
		return err
	}

	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		srv.port = tcpAddr.Port
		srv.httpServer.Addr = srv.Addr()
	}

	go func() {
		err := srv.httpServer.Serve(ln)
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

func (srv *Server) setupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/project", srv.handleProject)
	mux.HandleFunc("GET /api/plans", srv.handlePlans)
	mux.HandleFunc("GET /api/plans/{id}", srv.handlePlanByID)
	mux.HandleFunc("GET /api/issues", srv.handleIssues)
	mux.HandleFunc("GET /api/issues/{id}", srv.handleIssueByID)
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
