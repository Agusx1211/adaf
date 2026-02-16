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
	Host        string
	Port        int
	TLSMode     string
	CertFile    string
	KeyFile     string
	AuthToken   string
	RateLimit   float64
	RootDir     string
	AllowedRoot string
}

// Server hosts the HTTP API and WebSocket session stream bridge.
type Server struct {
	registry    *ProjectRegistry
	rootDir     string
	allowedRoot string
	httpServer  *http.Server
	port        int
	host        string
	tlsMode     string
	certFile    string
	keyFile     string
	authToken   string
	rateLimit   float64
}

// NewMulti constructs a web server with a pre-populated project registry.
func NewMulti(registry *ProjectRegistry, opts Options) *Server {
	return newServer(registry, opts)
}

func newServer(registry *ProjectRegistry, opts Options) *Server {
	host := strings.TrimSpace(opts.Host)
	if host == "" {
		host = "127.0.0.1"
	}

	port := opts.Port
	if port <= 0 {
		port = 8080
	}

	srv := &Server{
		registry:    registry,
		rootDir:     opts.RootDir,
		allowedRoot: opts.AllowedRoot,
		host:        host,
		port:        port,
		tlsMode:     strings.TrimSpace(opts.TLSMode),
		certFile:    strings.TrimSpace(opts.CertFile),
		keyFile:     strings.TrimSpace(opts.KeyFile),
		authToken:   strings.TrimSpace(opts.AuthToken),
		rateLimit:   opts.RateLimit,
	}
	if srv.allowedRoot == "" {
		srv.allowedRoot = srv.rootDir
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

// resolveProjectStore extracts the project store from a request.
// For project-scoped routes (/api/projects/{projectID}/...), it looks up
// the registry. For legacy routes (/api/...), it returns the default store.
func (srv *Server) resolveProjectStore(r *http.Request) (*store.Store, string, bool) {
	projectID := r.PathValue("projectID")
	if projectID != "" {
		s, ok := srv.registry.Get(projectID)
		if !ok {
			return nil, projectID, false
		}
		return s, projectID, true
	}
	// Legacy route — use default store from registry
	s, _ := srv.registry.Default()
	if s == nil {
		return nil, "", false
	}
	return s, "", true
}

// defaultStore returns the default project store from the registry.
func (srv *Server) defaultStore() *store.Store {
	s, _ := srv.registry.Default()
	return s
}

// projectHandler wraps a handler that needs a project store, resolving
// it from either the project-scoped or legacy route.
func (srv *Server) projectHandler(handler func(*store.Store, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, projectID, ok := srv.resolveProjectStore(r)
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", projectID))
			return
		}
		handler(s, w, r)
	}
}

func (srv *Server) setupRoutes(mux *http.ServeMux) {
	// Multi-project management endpoints
	mux.HandleFunc("GET /api/projects", srv.handleListProjects)
	mux.HandleFunc("GET /api/projects/dashboard", srv.handleGlobalDashboard)

	// Register project-scoped routes under /api/projects/{projectID}/...
	srv.registerProjectRoutes(mux, "/api/projects/{projectID}")

	// Register legacy routes under /api/... (backward compat, uses default project)
	srv.registerProjectRoutes(mux, "/api")

	// Filesystem browsing endpoints
	mux.HandleFunc("GET /api/fs/browse", srv.handleFSBrowse)
	mux.HandleFunc("POST /api/fs/mkdir", srv.handleFSMkdir)
	mux.HandleFunc("POST /api/projects/init", srv.handleProjectInit)
	mux.HandleFunc("POST /api/projects/open", srv.handleProjectOpen)

	// Config endpoints (global, not project-scoped)
	mux.HandleFunc("GET /api/config", srv.handleConfig)
	mux.HandleFunc("GET /api/config/profiles", srv.handleListProfiles)
	mux.HandleFunc("POST /api/config/profiles", srv.handleCreateProfile)
	mux.HandleFunc("PUT /api/config/profiles/{name}", srv.handleUpdateProfile)
	mux.HandleFunc("DELETE /api/config/profiles/{name}", srv.handleDeleteProfile)
	mux.HandleFunc("GET /api/config/loops", srv.handleListLoopDefs)
	mux.HandleFunc("POST /api/config/loops", srv.handleCreateLoopDef)
	mux.HandleFunc("PUT /api/config/loops/{name}", srv.handleUpdateLoopDef)
	mux.HandleFunc("DELETE /api/config/loops/{name}", srv.handleDeleteLoopDef)
	mux.HandleFunc("GET /api/config/roles", srv.handleListRoles)
	mux.HandleFunc("POST /api/config/roles", srv.handleCreateRole)
	mux.HandleFunc("PUT /api/config/roles/{name}", srv.handleUpdateRole)
	mux.HandleFunc("DELETE /api/config/roles/{name}", srv.handleDeleteRole)
	mux.HandleFunc("GET /api/config/rules", srv.handleListRules)
	mux.HandleFunc("POST /api/config/rules", srv.handleCreateRule)
	mux.HandleFunc("DELETE /api/config/rules/{id}", srv.handleDeleteRule)
	mux.HandleFunc("GET /api/config/teams", srv.handleListTeams)
	mux.HandleFunc("POST /api/config/teams", srv.handleCreateTeam)
	mux.HandleFunc("PUT /api/config/teams/{name}", srv.handleUpdateTeam)
	mux.HandleFunc("DELETE /api/config/teams/{name}", srv.handleDeleteTeam)
	mux.HandleFunc("GET /api/config/recent-combinations", srv.handleListRecentCombinations)
	mux.HandleFunc("GET /api/config/pushover", srv.handleGetPushover)
	mux.HandleFunc("PUT /api/config/pushover", srv.handleUpdatePushover)
	mux.HandleFunc("GET /api/config/skills", srv.handleListSkills)
	mux.HandleFunc("POST /api/config/skills", srv.handleCreateSkill)
	mux.HandleFunc("PUT /api/config/skills/{id}", srv.handleUpdateSkill)
	mux.HandleFunc("DELETE /api/config/skills/{id}", srv.handleDeleteSkill)
	mux.HandleFunc("GET /api/config/agents", srv.handleListAgents)
	mux.HandleFunc("POST /api/config/agents/detect", srv.handleDetectAgents)

	// Usage endpoints (global, not project-scoped)
	mux.HandleFunc("GET /api/usage", srv.handleUsage)

	// WebSocket endpoints
	mux.HandleFunc("GET /ws/sessions/{id}", srv.handleSessionWebSocket)
	mux.HandleFunc("GET /ws/terminal", srv.handleTerminalWebSocket)

	// Catch-all for unknown API routes
	mux.HandleFunc("GET /api/{rest...}", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	// Static files
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

// registerProjectRoutes registers all project-scoped API routes under a given prefix.
// The prefix is either "/api/projects/{projectID}" or "/api" (for backward compat).
func (srv *Server) registerProjectRoutes(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/project", srv.projectHandler(handleProjectP))

	mux.HandleFunc("GET "+prefix+"/plans", srv.projectHandler(handlePlansP))
	mux.HandleFunc("GET "+prefix+"/plans/{id}", srv.projectHandler(handlePlanByIDP))
	mux.HandleFunc("POST "+prefix+"/plans", srv.projectHandler(handleCreatePlanP))
	mux.HandleFunc("PUT "+prefix+"/plans/{id}", srv.projectHandler(handleUpdatePlanP))
	mux.HandleFunc("PUT "+prefix+"/plans/{id}/phases/{phaseId}", srv.projectHandler(handleUpdatePlanPhaseP))
	mux.HandleFunc("POST "+prefix+"/plans/{id}/activate", srv.projectHandler(handleActivatePlanP))
	mux.HandleFunc("DELETE "+prefix+"/plans/{id}", srv.projectHandler(handleDeletePlanP))

	mux.HandleFunc("GET "+prefix+"/issues", srv.projectHandler(handleIssuesP))
	mux.HandleFunc("GET "+prefix+"/issues/{id}", srv.projectHandler(handleIssueByIDP))
	mux.HandleFunc("POST "+prefix+"/issues", srv.projectHandler(handleCreateIssueP))
	mux.HandleFunc("PUT "+prefix+"/issues/{id}", srv.projectHandler(handleUpdateIssueP))
	mux.HandleFunc("DELETE "+prefix+"/issues/{id}", srv.projectHandler(handleDeleteIssueP))

	mux.HandleFunc("GET "+prefix+"/docs", srv.projectHandler(handleDocsP))
	mux.HandleFunc("GET "+prefix+"/docs/{id}", srv.projectHandler(handleDocByIDP))
	mux.HandleFunc("POST "+prefix+"/docs", srv.projectHandler(handleCreateDocP))
	mux.HandleFunc("PUT "+prefix+"/docs/{id}", srv.projectHandler(handleUpdateDocP))
	mux.HandleFunc("DELETE "+prefix+"/docs/{id}", srv.projectHandler(handleDeleteDocP))

	mux.HandleFunc("GET "+prefix+"/turns", srv.projectHandler(handleTurnsP))
	mux.HandleFunc("GET "+prefix+"/turns/{id}", srv.projectHandler(handleTurnByIDP))
	mux.HandleFunc("PUT "+prefix+"/turns/{id}", srv.projectHandler(handleUpdateTurnP))
	mux.HandleFunc("GET "+prefix+"/turns/{id}/events", srv.projectHandler(handleTurnRecordingEventsP))
	mux.HandleFunc("GET "+prefix+"/spawns", srv.projectHandler(handleSpawnsP))
	mux.HandleFunc("GET "+prefix+"/spawns/{id}", srv.projectHandler(handleSpawnByIDP))

	// Session control (project-scoped for create, since the store determines context)
	mux.HandleFunc("POST "+prefix+"/sessions/ask", srv.projectHandler(handleStartAskSessionP))
	mux.HandleFunc("POST "+prefix+"/sessions/loop", srv.projectHandler(handleStartLoopSessionP))
	mux.HandleFunc("POST "+prefix+"/sessions/{id}/stop", srv.projectHandler(handleStopSessionP))
	mux.HandleFunc("POST "+prefix+"/sessions/{id}/message", srv.projectHandler(handleSessionMessageP))
	mux.HandleFunc("GET "+prefix+"/sessions", srv.projectHandler(handleSessionsP))
	mux.HandleFunc("GET "+prefix+"/sessions/{id}", srv.projectHandler(handleSessionByIDP))

	// Loop runs
	mux.HandleFunc("GET "+prefix+"/loops", srv.projectHandler(handleLoopRunsP))
	mux.HandleFunc("GET "+prefix+"/loops/{id}", srv.projectHandler(handleLoopRunByIDP))
	mux.HandleFunc("GET "+prefix+"/loops/{id}/messages", srv.projectHandler(handleLoopMessagesP))
	mux.HandleFunc("POST "+prefix+"/loops/{id}/stop", srv.projectHandler(handleStopLoopRunP))
	mux.HandleFunc("POST "+prefix+"/loops/{id}/message", srv.projectHandler(handleLoopRunMessageP))

	// Chat Instances

	mux.HandleFunc("GET "+prefix+"/chat-instances", srv.projectHandler(handleListChatInstances))
	mux.HandleFunc("POST "+prefix+"/chat-instances", srv.projectHandler(handleCreateChatInstance))
	mux.HandleFunc("GET "+prefix+"/chat-instances/{id}", srv.projectHandler(handleGetChatInstanceMessages))
	mux.HandleFunc("POST "+prefix+"/chat-instances/{id}", srv.projectHandler(handleSendChatInstanceMessage))
	mux.HandleFunc("POST "+prefix+"/chat-instances/{id}/response", srv.projectHandler(handleSaveChatInstanceResponse))
	mux.HandleFunc("PATCH "+prefix+"/chat-instances/{id}", srv.projectHandler(handleUpdateChatInstance))
	mux.HandleFunc("DELETE "+prefix+"/chat-instances/{id}", srv.projectHandler(handleDeleteChatInstance))
	mux.HandleFunc("POST "+prefix+"/ui/missing-samples", srv.projectHandler(handleReportMissingUISampleP))

	// Stats
	mux.HandleFunc("GET "+prefix+"/stats/loops", srv.projectHandler(handleLoopStatsP))
	mux.HandleFunc("GET "+prefix+"/stats/profiles", srv.projectHandler(handleProfileStatsP))
}
