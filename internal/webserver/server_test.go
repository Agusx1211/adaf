package webserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test-project", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	return New(s, "127.0.0.1", 8080), s
}

func performRequest(t *testing.T, srv *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func decodeResponse[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestProjectEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	cfg := &store.ProjectConfig{Name: "web-project", RepoPath: "/tmp/web-project"}
	if err := s.SaveProject(cfg); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/project")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content-type = %q, want application/json", contentType)
	}

	got := decodeResponse[store.ProjectConfig](t, rec)
	if got.Name != cfg.Name {
		t.Fatalf("project name = %q, want %q", got.Name, cfg.Name)
	}
	if got.RepoPath != cfg.RepoPath {
		t.Fatalf("project repo_path = %q, want %q", got.RepoPath, cfg.RepoPath)
	}
}

func TestPlansEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	if err := s.SavePlan(&store.Plan{ID: "default", Title: "Default Plan", Status: "active"}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/plans")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	plans := decodeResponse[[]store.Plan](t, rec)
	if len(plans) != 1 {
		t.Fatalf("plans length = %d, want 1", len(plans))
	}
	if plans[0].ID != "default" {
		t.Fatalf("plan id = %q, want %q", plans[0].ID, "default")
	}
}

func TestPlanByIDEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	if err := s.SavePlan(&store.Plan{ID: "core", Title: "Core Plan", Status: "active"}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/plans/core")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	plan := decodeResponse[store.Plan](t, rec)
	if plan.ID != "core" {
		t.Fatalf("plan id = %q, want %q", plan.ID, "core")
	}

	notFound := performRequest(t, srv, http.MethodGet, "/api/plans/missing")
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", notFound.Code, http.StatusNotFound)
	}
}

func TestIssuesEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	if err := s.CreateIssue(&store.Issue{Title: "Open", Status: "open", Priority: "high", PlanID: "default"}); err != nil {
		t.Fatalf("CreateIssue open: %v", err)
	}
	if err := s.CreateIssue(&store.Issue{Title: "Resolved", Status: "resolved", Priority: "low", PlanID: "other"}); err != nil {
		t.Fatalf("CreateIssue resolved: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/issues")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	all := decodeResponse[[]store.Issue](t, rec)
	if len(all) != 2 {
		t.Fatalf("issues length = %d, want 2", len(all))
	}

	filteredRec := performRequest(t, srv, http.MethodGet, "/api/issues?status=open")
	if filteredRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", filteredRec.Code, http.StatusOK)
	}

	filtered := decodeResponse[[]store.Issue](t, filteredRec)
	if len(filtered) != 1 {
		t.Fatalf("filtered issues length = %d, want 1", len(filtered))
	}
	if filtered[0].Status != "open" {
		t.Fatalf("filtered status = %q, want %q", filtered[0].Status, "open")
	}
}

func TestTurnsEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	if err := s.CreateTurn(&store.Turn{Agent: "claude", Objective: "Turn one"}); err != nil {
		t.Fatalf("CreateTurn #1: %v", err)
	}
	if err := s.CreateTurn(&store.Turn{Agent: "codex", Objective: "Turn two"}); err != nil {
		t.Fatalf("CreateTurn #2: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/turns")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	turns := decodeResponse[[]store.Turn](t, rec)
	if len(turns) != 2 {
		t.Fatalf("turns length = %d, want 2", len(turns))
	}
}

func TestSessionsEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/api/sessions")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	sessions := decodeResponse[[]session.SessionMeta](t, rec)
	if len(sessions) != 0 {
		t.Fatalf("sessions length = %d, want 0", len(sessions))
	}
}

func TestCORS(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/api/project")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "GET") {
		t.Fatalf("Access-Control-Allow-Methods = %q, expected to contain GET", got)
	}

	preflight := performRequest(t, srv, http.MethodOptions, "/api/project")
	if preflight.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", preflight.Code, http.StatusNoContent)
	}
}

func TestNotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/api/nonexistent")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStaticFilesServed(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<html") {
		t.Fatal("GET / did not return HTML")
	}

	rec = performRequest(t, srv, http.MethodGet, "/static/style.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/style.css status = %d, want %d", rec.Code, http.StatusOK)
	}

	rec = performRequest(t, srv, http.MethodGet, "/static/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/app.js status = %d, want %d", rec.Code, http.StatusOK)
	}
}
