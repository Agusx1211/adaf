package webserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/config"
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

	registry := NewProjectRegistry()
	if err := registry.Register("test-project", projectDir); err != nil {
		t.Fatalf("registry.Register: %v", err)
	}

	return NewMulti(registry, Options{RootDir: projectDir}), s
}

func performRequest(t *testing.T, srv *Server, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec
}

func performJSONRequest(t *testing.T, srv *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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
	if err := s.CreateIssue(&store.Issue{Title: "Closed", Status: "closed", Priority: "low", PlanID: "other"}); err != nil {
		t.Fatalf("CreateIssue closed: %v", err)
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

func TestCreateAndUpdateIssueEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/issues", `{"title":"Fix API","description":"add write handlers","labels":["api"]}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	created := decodeResponse[store.Issue](t, createRec)
	if created.Title != "Fix API" {
		t.Fatalf("created title = %q, want %q", created.Title, "Fix API")
	}
	if created.Status != "open" {
		t.Fatalf("created status = %q, want %q", created.Status, "open")
	}
	if created.Priority != "medium" {
		t.Fatalf("created priority = %q, want %q", created.Priority, "medium")
	}

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/issues/"+strconv.Itoa(created.ID), `{"status":"closed","priority":"low","updated_by":"agent-a"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	updated := decodeResponse[store.Issue](t, updateRec)
	if updated.Status != "closed" {
		t.Fatalf("updated status = %q, want %q", updated.Status, "closed")
	}
	if updated.Priority != "low" {
		t.Fatalf("updated priority = %q, want %q", updated.Priority, "low")
	}
	if updated.UpdatedBy != "agent-a" {
		t.Fatalf("updated by = %q, want %q", updated.UpdatedBy, "agent-a")
	}
	if len(updated.History) == 0 {
		t.Fatalf("updated history length = %d, want > 0", len(updated.History))
	}
}

func TestIssueCommentEndpoint(t *testing.T) {
	srv, s := newTestServer(t)

	if err := s.CreateIssue(&store.Issue{Title: "Needs Notes", Status: "open", Priority: "medium"}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	createComment := performJSONRequest(t, srv, http.MethodPost, "/api/issues/1/comments", `{"body":"Working on this now","by":"agent-b"}`)
	if createComment.Code != http.StatusCreated {
		t.Fatalf("comment status = %d, want %d", createComment.Code, http.StatusCreated)
	}

	updated := decodeResponse[store.Issue](t, createComment)
	if len(updated.Comments) != 1 {
		t.Fatalf("comment count = %d, want 1", len(updated.Comments))
	}
	if updated.Comments[0].By != "agent-b" {
		t.Fatalf("comment by = %q, want %q", updated.Comments[0].By, "agent-b")
	}
	if len(updated.History) < 2 {
		t.Fatalf("history length = %d, want >= 2", len(updated.History))
	}
	if updated.History[len(updated.History)-1].Type != "commented" {
		t.Fatalf("last history type = %q, want %q", updated.History[len(updated.History)-1].Type, "commented")
	}
}

func TestPlanWriteEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/plans", `{"id":"web-ui","title":"Web UI","description":"overhaul"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	created := decodeResponse[store.Plan](t, createRec)
	if created.Status != "active" {
		t.Fatalf("created status = %q, want %q", created.Status, "active")
	}

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/plans/web-ui", `{"title":"Web UI v2","status":"frozen"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	updated := decodeResponse[store.Plan](t, updateRec)
	if updated.Title != "Web UI v2" {
		t.Fatalf("updated title = %q, want %q", updated.Title, "Web UI v2")
	}
	if updated.Status != "frozen" {
		t.Fatalf("updated status = %q, want %q", updated.Status, "frozen")
	}

	activateRec := performRequest(t, srv, http.MethodPost, "/api/plans/web-ui/activate")
	if activateRec.Code != http.StatusOK {
		t.Fatalf("activate status = %d, want %d", activateRec.Code, http.StatusOK)
	}

	var activateOut map[string]bool
	if err := json.NewDecoder(activateRec.Body).Decode(&activateOut); err != nil {
		t.Fatalf("decode activate response: %v", err)
	}
	if !activateOut["ok"] {
		t.Fatal("activate response missing ok=true")
	}

	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/plans/web-ui")
	if deleteRec.Code != http.StatusBadRequest {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusBadRequest)
	}

	doneRec := performJSONRequest(t, srv, http.MethodPut, "/api/plans/web-ui", `{"status":"done"}`)
	if doneRec.Code != http.StatusOK {
		t.Fatalf("set done status = %d, want %d", doneRec.Code, http.StatusOK)
	}

	deleteRec = performRequest(t, srv, http.MethodDelete, "/api/plans/web-ui")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}

	missingRec := performRequest(t, srv, http.MethodGet, "/api/plans/web-ui")
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("plan after delete status = %d, want %d", missingRec.Code, http.StatusNotFound)
	}
}

func TestWikiEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/wiki", `{"plan_id":"web-ui","title":"Architecture Draft","content":"v1","updated_by":"lead-agent"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	created := decodeResponse[store.WikiEntry](t, createRec)
	if created.ID != "architecture-draft" {
		t.Fatalf("created id = %q, want %q", created.ID, "architecture-draft")
	}
	if created.PlanID != "web-ui" {
		t.Fatalf("created plan_id = %q, want %q", created.PlanID, "web-ui")
	}
	if created.CreatedBy != "lead-agent" {
		t.Fatalf("created_by = %q, want %q", created.CreatedBy, "lead-agent")
	}
	if created.UpdatedBy != "lead-agent" {
		t.Fatalf("updated_by = %q, want %q", created.UpdatedBy, "lead-agent")
	}
	if created.Version != 1 {
		t.Fatalf("version = %d, want 1", created.Version)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/wiki?plan=web-ui")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	listed := decodeResponse[[]store.WikiEntry](t, listRec)
	if len(listed) != 1 {
		t.Fatalf("wiki length = %d, want 1", len(listed))
	}

	getRec := performRequest(t, srv, http.MethodGet, "/api/wiki/architecture-draft")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/wiki/architecture-draft", `{"content":"v2","updated_by":"worker-agent"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}
	updated := decodeResponse[store.WikiEntry](t, updateRec)
	if updated.Content != "v2" {
		t.Fatalf("updated content = %q, want %q", updated.Content, "v2")
	}
	if updated.UpdatedBy != "worker-agent" {
		t.Fatalf("updated_by = %q, want %q", updated.UpdatedBy, "worker-agent")
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}

	searchRec := performRequest(t, srv, http.MethodGet, "/api/wiki/search?q=arch")
	if searchRec.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d", searchRec.Code, http.StatusOK)
	}
	searchResults := decodeResponse[[]store.WikiEntry](t, searchRec)
	if len(searchResults) != 1 {
		t.Fatalf("search length = %d, want 1", len(searchResults))
	}
	if searchResults[0].ID != "architecture-draft" {
		t.Fatalf("top search result = %q, want %q", searchResults[0].ID, "architecture-draft")
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

func TestLoopRunsEndpoint_ReconcilesDeadDaemonSession(t *testing.T) {
	srv, s := newTestServer(t)

	projectCfg, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	sessionID, err := session.CreateSession(session.DaemonConfig{
		ProjectDir:  s.ProjectDir(),
		ProjectName: projectCfg.Name,
		WorkDir:     s.ProjectDir(),
		ProfileName: "p1",
		AgentName:   "codex",
		Loop: config.LoopDef{
			Name: "stale-loop",
			Steps: []config.LoopStep{
				{Profile: "p1", Turns: 1},
			},
		},
		Profiles: []config.Profile{{Name: "p1", Agent: "codex"}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	meta, err := session.LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	meta.Status = session.StatusDead
	meta.EndedAt = time.Now().UTC().Add(-1 * time.Minute)
	meta.Error = "daemon process died unexpectedly"
	if err := session.SaveMeta(sessionID, meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	run := &store.LoopRun{
		LoopName:        "stale-loop",
		Status:          "running",
		DaemonSessionID: sessionID,
		Steps: []store.LoopRunStep{
			{Profile: "p1", Turns: 1},
		},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/loops")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	runs := decodeResponse[[]store.LoopRun](t, rec)
	if len(runs) != 1 {
		t.Fatalf("runs length = %d, want 1", len(runs))
	}
	if runs[0].ID != run.ID {
		t.Fatalf("run id = %d, want %d", runs[0].ID, run.ID)
	}
	if runs[0].Status != "stopped" {
		t.Fatalf("run status = %q, want %q", runs[0].Status, "stopped")
	}
	if runs[0].StoppedAt.IsZero() {
		t.Fatal("run stopped_at is zero, want populated timestamp")
	}

	persisted, err := s.GetLoopRun(run.ID)
	if err != nil {
		t.Fatalf("GetLoopRun: %v", err)
	}
	if persisted.Status != "stopped" {
		t.Fatalf("persisted run status = %q, want %q", persisted.Status, "stopped")
	}
	if persisted.StoppedAt.IsZero() {
		t.Fatal("persisted run stopped_at is zero, want populated timestamp")
	}
}

func TestSessionRecordingEventsEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	sessionID := 42
	if err := os.MkdirAll(session.SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll(session dir): %v", err)
	}
	want := "{\"type\":\"raw\",\"data\":\"hello\"}\n"
	if err := os.WriteFile(session.EventsPath(sessionID), []byte(want), 0644); err != nil {
		t.Fatalf("WriteFile(session events): %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/sessions/42/events")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/x-ndjson") {
		t.Fatalf("content-type = %q, want application/x-ndjson", got)
	}
	if rec.Body.String() != want {
		t.Fatalf("body = %q, want %q", rec.Body.String(), want)
	}
}

func TestSessionRecordingEventsEndpointTailQuery(t *testing.T) {
	srv, _ := newTestServer(t)

	sessionID := 43
	if err := os.MkdirAll(session.SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll(session dir): %v", err)
	}
	all := strings.Join([]string{
		`{"type":"raw","data":"line-1"}`,
		`{"type":"raw","data":"line-2"}`,
		`{"type":"raw","data":"line-3"}`,
		`{"type":"raw","data":"line-4"}`,
		"",
	}, "\n")
	if err := os.WriteFile(session.EventsPath(sessionID), []byte(all), 0644); err != nil {
		t.Fatalf("WriteFile(session events): %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/api/sessions/43/events?tail=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := strings.Join([]string{
		`{"type":"raw","data":"line-3"}`,
		`{"type":"raw","data":"line-4"}`,
		"",
	}, "\n")
	if rec.Body.String() != want {
		t.Fatalf("body = %q, want %q", rec.Body.String(), want)
	}
}

func TestSessionRecordingEventsEndpointRejectsInvalidTailQuery(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/api/sessions/42/events?tail=oops")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteIssueEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create issue first
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/issues", `{"title":"To Delete","priority":"low"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	created := decodeResponse[store.Issue](t, createRec)

	// Delete it
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/issues/"+strconv.Itoa(created.ID))
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}

	// Verify it's gone
	getRec := performRequest(t, srv, http.MethodGet, "/api/issues/"+strconv.Itoa(created.ID))
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}

	// Delete non-existent
	notFoundRec := performRequest(t, srv, http.MethodDelete, "/api/issues/999")
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("delete non-existent status = %d, want %d", notFoundRec.Code, http.StatusNotFound)
	}
}

func TestDeleteWikiEntryEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create wiki entry first
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/wiki", `{"title":"Temp Wiki","content":"delete me"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	created := decodeResponse[store.WikiEntry](t, createRec)

	// Delete it
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/wiki/"+created.ID)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}

	// Verify it's gone
	getRec := performRequest(t, srv, http.MethodGet, "/api/wiki/"+created.ID)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}

	// Delete non-existent
	notFoundRec := performRequest(t, srv, http.MethodDelete, "/api/wiki/no-such-wiki")
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("delete non-existent status = %d, want %d", notFoundRec.Code, http.StatusNotFound)
	}
}

func TestConfigProfileEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	// List profiles (empty initially)
	listRec := performRequest(t, srv, http.MethodGet, "/api/config/profiles")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	// Create profile
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"test-prof","agent":"claude","model":"sonnet","cost":"cheap"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	// Verify it appears in list
	listRec = performRequest(t, srv, http.MethodGet, "/api/config/profiles")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list after create status = %d, want %d", listRec.Code, http.StatusOK)
	}

	// Update profile
	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/profiles/test-prof", `{"name":"ignored-by-path","agent":"claude","model":"opus","cost":"expensive"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	listRec = performRequest(t, srv, http.MethodGet, "/api/config/profiles")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list after update status = %d, want %d", listRec.Code, http.StatusOK)
	}
	profiles := decodeResponse[[]config.Profile](t, listRec)
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	if profiles[0].Name != "test-prof" {
		t.Fatalf("profile name = %q, want %q", profiles[0].Name, "test-prof")
	}
	if profiles[0].Cost != "expensive" {
		t.Fatalf("profile cost = %q, want %q", profiles[0].Cost, "expensive")
	}

	// Delete profile
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/config/profiles/test-prof")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}
}

func TestConfigProfileRejectsInvalidCost(t *testing.T) {
	srv, _ := newTestServer(t)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"test-prof","agent":"claude","cost":"premium"}`)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusBadRequest)
	}

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"test-prof","agent":"claude","cost":"cheap"}`)
	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/profiles/test-prof", `{"name":"test-prof","agent":"claude","cost":"ultra"}`)
	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusBadRequest)
	}
}

func TestConfigLoopDefEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create a profile first (needed for loop step)
	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"loop-prof","agent":"claude","model":"sonnet"}`)

	// Create loop
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops", `{"name":"test-loop","steps":[{"profile":"loop-prof","turns":1}]}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	// List loops
	listRec := performRequest(t, srv, http.MethodGet, "/api/config/loops")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	// Update loop
	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/loops/test-loop", `{"name":"test-loop","steps":[{"profile":"loop-prof","turns":2}]}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	// Delete loop
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/config/loops/test-loop")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}
}

func TestConfigLoopDefRejectsInvalidResourcePriority(t *testing.T) {
	srv, _ := newTestServer(t)
	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"loop-prof","agent":"claude"}`)

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops", `{
		"name":"bad-loop",
		"resource_priority":"fast",
		"steps":[{"profile":"loop-prof","turns":1}]
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestConfigLoopDefUpdateClearsOptionalStepFields(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"loop-prof","agent":"claude"}`)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops", `{
		"name":"test-loop",
		"steps":[{"profile":"loop-prof","turns":1,"team":"my-team","instructions":"hello","manual_prompt":"manual hello"}]
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/loops/test-loop", `{
		"name":"test-loop",
		"steps":[{"profile":"loop-prof","turns":2}]
	}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/config/loops")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	loops := decodeResponse[[]config.LoopDef](t, listRec)
	if len(loops) != 1 {
		t.Fatalf("loops length = %d, want 1", len(loops))
	}
	if len(loops[0].Steps) != 1 {
		t.Fatalf("steps length = %d, want 1", len(loops[0].Steps))
	}
	if loops[0].Steps[0].Team != "" {
		t.Fatalf("step team = %q, want empty", loops[0].Steps[0].Team)
	}
	if loops[0].Steps[0].Instructions != "" {
		t.Fatalf("step instructions = %q, want empty", loops[0].Steps[0].Instructions)
	}
	if loops[0].Steps[0].ManualPrompt != "" {
		t.Fatalf("step manual_prompt = %q, want empty", loops[0].Steps[0].ManualPrompt)
	}
}

func TestConfigLoopDefPersistsExplicitNoSkills(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"loop-prof","agent":"claude"}`)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops", `{
		"name":"skills-loop",
		"steps":[{"profile":"loop-prof","skills":[],"skills_explicit":true}]
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/config/loops")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	loops := decodeResponse[[]config.LoopDef](t, listRec)
	if len(loops) != 1 {
		t.Fatalf("loops length = %d, want 1", len(loops))
	}
	if len(loops[0].Steps) != 1 {
		t.Fatalf("steps length = %d, want 1", len(loops[0].Steps))
	}
	step := loops[0].Steps[0]
	if !step.SkillsExplicit {
		t.Fatal("step skills_explicit = false, want true")
	}
	if len(step.Skills) != 0 {
		t.Fatalf("step skills length = %d, want 0", len(step.Skills))
	}
}

func TestLoopPromptPreviewEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"preview-prof","agent":"generic"}`)

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops/prompt-preview", `{
		"loop":{
			"name":"preview-loop",
			"steps":[
				{"profile":"preview-prof","position":"lead","turns":2,"instructions":"Implement the selected step objective.","can_message":true}
			]
		},
		"step_index":0
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d", rec.Code, http.StatusOK)
	}

	var out struct {
		RuntimePath string `json:"runtime_path"`
		LoopName    string `json:"loop_name"`
		Profile     string `json:"profile"`
		Position    string `json:"position"`
		Scenarios   []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
			Exact  bool   `json:"exact"`
		} `json:"scenarios"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if out.RuntimePath != "looprun.BuildStepPrompt + loop.BuildResumePrompt" {
		t.Fatalf("runtime_path = %q, want %q", out.RuntimePath, "looprun.BuildStepPrompt + loop.BuildResumePrompt")
	}
	if out.LoopName != "preview-loop" {
		t.Fatalf("loop_name = %q, want %q", out.LoopName, "preview-loop")
	}
	if out.Profile != "preview-prof" {
		t.Fatalf("profile = %q, want %q", out.Profile, "preview-prof")
	}
	if out.Position != config.PositionLead {
		t.Fatalf("position = %q, want %q", out.Position, config.PositionLead)
	}
	if len(out.Scenarios) != 2 {
		t.Fatalf("scenario count = %d, want 2", len(out.Scenarios))
	}

	var freshPrompt, resumePrompt string
	for _, sc := range out.Scenarios {
		if !sc.Exact {
			t.Fatalf("scenario %q exact = false, want true", sc.ID)
		}
		switch sc.ID {
		case "fresh_turn":
			freshPrompt = sc.Prompt
		case "resume_turn":
			resumePrompt = sc.Prompt
		}
	}
	if freshPrompt == "" {
		t.Fatal("fresh_turn prompt missing")
	}
	if !strings.Contains(freshPrompt, "Project: test-project") {
		t.Fatalf("fresh prompt missing project context:\n%s", freshPrompt)
	}
	if !strings.Contains(freshPrompt, "Implement the selected step objective.") {
		t.Fatalf("fresh prompt missing step instructions:\n%s", freshPrompt)
	}
	if !strings.Contains(resumePrompt, "Continue from where you left off.") {
		t.Fatalf("resume prompt = %q, want continuation lead", resumePrompt)
	}
}

func TestLoopPromptPreviewEndpoint_ManualPromptBypassesBuilder(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"preview-prof","agent":"generic"}`)

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops/prompt-preview", `{
		"loop":{
			"name":"preview-loop",
			"steps":[
				{"profile":"preview-prof","position":"lead","turns":1,"instructions":"ignored","manual_prompt":"manual override prompt"}
			]
		},
		"step_index":0
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d", rec.Code, http.StatusOK)
	}

	var out struct {
		Scenarios []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
		} `json:"scenarios"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode preview: %v", err)
	}

	var freshPrompt string
	for _, sc := range out.Scenarios {
		if sc.ID == "fresh_turn" {
			freshPrompt = sc.Prompt
		}
	}
	if freshPrompt != "manual override prompt" {
		t.Fatalf("fresh prompt = %q, want manual override", freshPrompt)
	}
	if strings.Contains(freshPrompt, "Project: test-project") {
		t.Fatalf("manual prompt should bypass auto-built context:\n%s", freshPrompt)
	}
}

func TestStartLoopSessionRejectsInvalidPriority(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"loop-prof","agent":"generic"}`)
	performJSONRequest(t, srv, http.MethodPost, "/api/config/loops", `{"name":"test-loop","steps":[{"profile":"loop-prof","position":"lead","turns":1}]}`)

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/sessions/loop", `{
		"loop":"test-loop",
		"priority":"fast"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("start status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestTeamPromptPreviewEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"worker-prof","agent":"generic"}`)

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/config/teams/prompt-preview", `{
		"child_profile":"worker-prof",
		"child_role":"developer",
		"team":{
			"name":"preview-team",
			"delegation":{
				"profiles":[
					{"name":"worker-prof","roles":["developer"]}
				]
			}
		}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d", rec.Code, http.StatusOK)
	}

	var out struct {
		RuntimePath string `json:"runtime_path"`
		TeamName    string `json:"team_name"`
		Profile     string `json:"profile"`
		Position    string `json:"position"`
		Role        string `json:"role"`
		Scenarios   []struct {
			ID     string `json:"id"`
			Prompt string `json:"prompt"`
			Exact  bool   `json:"exact"`
		} `json:"scenarios"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if out.RuntimePath != "prompt.Build (sub-agent) + loop.BuildResumePrompt" {
		t.Fatalf("runtime_path = %q, want %q", out.RuntimePath, "prompt.Build (sub-agent) + loop.BuildResumePrompt")
	}
	if out.TeamName != "preview-team" {
		t.Fatalf("team_name = %q, want %q", out.TeamName, "preview-team")
	}
	if out.Profile != "worker-prof" {
		t.Fatalf("profile = %q, want %q", out.Profile, "worker-prof")
	}
	if out.Position != config.PositionWorker {
		t.Fatalf("position = %q, want %q", out.Position, config.PositionWorker)
	}
	if out.Role != config.RoleDeveloper {
		t.Fatalf("role = %q, want %q", out.Role, config.RoleDeveloper)
	}
	if len(out.Scenarios) != 2 {
		t.Fatalf("scenario count = %d, want 2", len(out.Scenarios))
	}

	var freshPrompt, resumePrompt string
	for _, sc := range out.Scenarios {
		if !sc.Exact {
			t.Fatalf("scenario %q exact = false, want true", sc.ID)
		}
		switch sc.ID {
		case "fresh_turn":
			freshPrompt = sc.Prompt
		case "resume_turn":
			resumePrompt = sc.Prompt
		}
	}
	if freshPrompt == "" {
		t.Fatal("fresh_turn prompt missing")
	}
	if !strings.Contains(freshPrompt, "You are a sub-agent working as a developer.") {
		t.Fatalf("fresh prompt missing sub-agent intro:\n%s", freshPrompt)
	}
	if !strings.Contains(freshPrompt, "adaf parent-ask") {
		t.Fatalf("fresh prompt missing parent communication guidance:\n%s", freshPrompt)
	}
	if !strings.Contains(freshPrompt, "Preview task: implement the delegated sub-task") {
		t.Fatalf("fresh prompt missing preview task text:\n%s", freshPrompt)
	}
	if !strings.Contains(resumePrompt, "Continue from where you left off.") {
		t.Fatalf("resume prompt = %q, want continuation lead", resumePrompt)
	}
}

func TestConfigTeamUpdateClearsDelegation(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"sub-prof","agent":"claude"}`)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/teams", `{
		"name":"test-team",
		"delegation":{"profiles":[{"name":"sub-prof"}]}
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/teams/test-team", `{"name":"test-team"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/config/teams")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	teams := decodeResponse[[]config.Team](t, listRec)
	if len(teams) != 1 {
		t.Fatalf("teams length = %d, want 1", len(teams))
	}
	if teams[0].Delegation != nil {
		t.Fatalf("team delegation = %#v, want nil", teams[0].Delegation)
	}
}

func TestConfigTeamPersistsDelegationTimeoutMinutes(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"sub-prof","agent":"claude"}`)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/teams", `{
		"name":"timeout-team",
		"delegation":{"profiles":[{"name":"sub-prof","timeout_minutes":7}]}
	}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/config/teams")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	teams := decodeResponse[[]config.Team](t, listRec)
	if len(teams) != 1 {
		t.Fatalf("teams length = %d, want 1", len(teams))
	}
	if teams[0].Delegation == nil || len(teams[0].Delegation.Profiles) != 1 {
		t.Fatalf("team delegation = %#v, want one profile", teams[0].Delegation)
	}
	if teams[0].Delegation.Profiles[0].TimeoutMinutes != 7 {
		t.Fatalf("delegation timeout_minutes = %d, want 7", teams[0].Delegation.Profiles[0].TimeoutMinutes)
	}
}

func TestConfigRolesEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	// List roles (includes defaults)
	listRec := performRequest(t, srv, http.MethodGet, "/api/config/roles")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	// Create role
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/roles", `{"name":"test-role","title":"Test Role","identity":"You are a test role.","can_write_code":true}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	// Delete role
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/config/roles/test-role")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}
}

func TestConfigRulesEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	// List rules
	listRec := performRequest(t, srv, http.MethodGet, "/api/config/rules")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}

	// Create rule
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/rules", `{"id":"test-rule","body":"Always test your code."}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	// Delete rule
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/config/rules/test-rule")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}
}

func TestConfigPushoverEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	// Get pushover (empty initially)
	getRec := performRequest(t, srv, http.MethodGet, "/api/config/pushover")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	// Update pushover
	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/pushover", `{"user_key":"test-user","app_token":"test-token"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}
}

func TestLoopRunEndpoints(t *testing.T) {
	srv, s := newTestServer(t)

	// List loop runs (empty)
	listRec := performRequest(t, srv, http.MethodGet, "/api/loops")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	runs := decodeResponse[[]store.LoopRun](t, listRec)
	if len(runs) != 0 {
		t.Fatalf("loop runs length = %d, want 0", len(runs))
	}

	// Get non-existent
	notFoundRec := performRequest(t, srv, http.MethodGet, "/api/loops/999")
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("get non-existent status = %d, want %d", notFoundRec.Code, http.StatusNotFound)
	}

	run := &store.LoopRun{
		LoopName:        "wind-down-endpoint-test",
		Status:          "running",
		StepLastSeenMsg: map[int]int{},
		Steps:           []store.LoopRunStep{{Profile: "p1", Turns: 1}},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun: %v", err)
	}

	windDownRec := performRequest(t, srv, http.MethodPost, "/api/loops/"+strconv.Itoa(run.ID)+"/wind-down")
	if windDownRec.Code != http.StatusOK {
		t.Fatalf("wind-down status = %d, want %d", windDownRec.Code, http.StatusOK)
	}
	if !s.IsLoopWindDown(run.ID) {
		t.Fatalf("IsLoopWindDown(%d) = false, want true", run.ID)
	}
}

func TestStatsEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	loopRec := performRequest(t, srv, http.MethodGet, "/api/stats/loops")
	if loopRec.Code != http.StatusOK {
		t.Fatalf("loop stats status = %d, want %d", loopRec.Code, http.StatusOK)
	}

	profileRec := performRequest(t, srv, http.MethodGet, "/api/stats/profiles")
	if profileRec.Code != http.StatusOK {
		t.Fatalf("profile stats status = %d, want %d", profileRec.Code, http.StatusOK)
	}
}

func TestProfilePerformanceEndpoints(t *testing.T) {
	srv, s := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"spark","agent":"codex","cost":"cheap"}`)

	spawn := &store.SpawnRecord{
		ParentTurnID:  10,
		ParentProfile: "manager-a",
		ChildProfile:  "spark",
		ChildRole:     "scout",
		ChildPosition: "worker",
		Task:          "Inspect failing tests",
		Status:        store.SpawnStatusRunning,
	}
	if err := s.CreateSpawn(spawn); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}
	saved, err := s.GetSpawn(spawn.ID)
	if err != nil {
		t.Fatalf("GetSpawn: %v", err)
	}
	saved.Status = store.SpawnStatusCompleted
	saved.StartedAt = time.Now().Add(-90 * time.Second).UTC()
	saved.CompletedAt = time.Now().UTC()
	if err := s.UpdateSpawn(saved); err != nil {
		t.Fatalf("UpdateSpawn: %v", err)
	}

	feedbackRec := performJSONRequest(t, srv, http.MethodPost, "/api/spawns/"+strconv.Itoa(spawn.ID)+"/feedback", `{"difficulty":6.5,"quality":8.5,"notes":"good scouting output","parent_role":"lead","parent_position":"manager"}`)
	if feedbackRec.Code != http.StatusOK {
		t.Fatalf("feedback status = %d, want %d", feedbackRec.Code, http.StatusOK)
	}
	feedback := decodeResponse[map[string]any](t, feedbackRec)
	if got := feedback["child_profile"]; got != "spark" {
		t.Fatalf("child_profile = %v, want spark", got)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/performance/profiles")
	if listRec.Code != http.StatusOK {
		t.Fatalf("performance list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	report := decodeResponse[map[string]any](t, listRec)
	profilesAny, ok := report["profiles"].([]any)
	if !ok || len(profilesAny) == 0 {
		t.Fatalf("profiles payload missing or empty: %#v", report["profiles"])
	}

	detailRec := performRequest(t, srv, http.MethodGet, "/api/performance/profiles/spark")
	if detailRec.Code != http.StatusOK {
		t.Fatalf("performance detail status = %d, want %d", detailRec.Code, http.StatusOK)
	}
	detail := decodeResponse[map[string]any](t, detailRec)
	summary, ok := detail["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary payload missing: %#v", detail)
	}
	if got := summary["profile"]; got != "spark" {
		t.Fatalf("summary.profile = %v, want spark", got)
	}
	if got := summary["cost"]; got != "cheap" {
		t.Fatalf("summary.cost = %v, want cheap", got)
	}
	records, ok := detail["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
}

func TestProfilePerformanceFeedbackRequiresTerminalSpawn(t *testing.T) {
	srv, s := newTestServer(t)

	spawn := &store.SpawnRecord{
		ParentTurnID:  10,
		ParentProfile: "manager-a",
		ChildProfile:  "spark",
		Status:        store.SpawnStatusRunning,
		Task:          "Do something",
	}
	if err := s.CreateSpawn(spawn); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/spawns/"+strconv.Itoa(spawn.ID)+"/feedback", `{"difficulty":5,"quality":8}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestIssueValidation(t *testing.T) {
	srv, _ := newTestServer(t)

	// Missing title
	rec := performJSONRequest(t, srv, http.MethodPost, "/api/issues", `{"description":"no title"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no title status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Invalid status
	rec = performJSONRequest(t, srv, http.MethodPost, "/api/issues", `{"title":"Bad","status":"invalid"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Invalid priority
	rec = performJSONRequest(t, srv, http.MethodPost, "/api/issues", `{"title":"Bad","priority":"extreme"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid priority = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Unknown dependency issue ID
	rec = performJSONRequest(t, srv, http.MethodPost, "/api/issues", `{"title":"Bad deps","depends_on":[999]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid dependencies = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPlanValidation(t *testing.T) {
	srv, _ := newTestServer(t)

	// Missing id
	rec := performJSONRequest(t, srv, http.MethodPost, "/api/plans", `{"title":"No ID"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no id status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Missing title
	rec = performJSONRequest(t, srv, http.MethodPost, "/api/plans", `{"id":"no-title"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no title status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	// Invalid plan status on update
	performJSONRequest(t, srv, http.MethodPost, "/api/plans", `{"id":"val-test","title":"Validation Test"}`)
	rec = performJSONRequest(t, srv, http.MethodPut, "/api/plans/val-test", `{"status":"bogus"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid plan status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

}

func TestWikiValidation(t *testing.T) {
	srv, _ := newTestServer(t)

	// Missing title
	rec := performJSONRequest(t, srv, http.MethodPost, "/api/wiki", `{"content":"no title"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no title status = %d, want %d", rec.Code, http.StatusBadRequest)
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
