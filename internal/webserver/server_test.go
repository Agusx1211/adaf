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

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/issues/"+strconv.Itoa(created.ID), `{"status":"resolved","priority":"low"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	updated := decodeResponse[store.Issue](t, updateRec)
	if updated.Status != "resolved" {
		t.Fatalf("updated status = %q, want %q", updated.Status, "resolved")
	}
	if updated.Priority != "low" {
		t.Fatalf("updated priority = %q, want %q", updated.Priority, "low")
	}
}

func TestPlanWriteEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/plans", `{"id":"web-ui","title":"Web UI","description":"overhaul","phases":[{"id":"phase-1","title":"MVP","status":"not_started","priority":1}]}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	created := decodeResponse[store.Plan](t, createRec)
	if created.Status != "active" {
		t.Fatalf("created status = %q, want %q", created.Status, "active")
	}
	if len(created.Phases) != 1 {
		t.Fatalf("created phase count = %d, want 1", len(created.Phases))
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

	phaseRec := performJSONRequest(t, srv, http.MethodPut, "/api/plans/web-ui/phases/phase-1", `{"status":"in_progress","priority":3,"depends_on":["phase-0"]}`)
	if phaseRec.Code != http.StatusOK {
		t.Fatalf("phase update status = %d, want %d", phaseRec.Code, http.StatusOK)
	}

	phaseUpdated := decodeResponse[store.Plan](t, phaseRec)
	if phaseUpdated.Phases[0].Status != "in_progress" {
		t.Fatalf("phase status = %q, want %q", phaseUpdated.Phases[0].Status, "in_progress")
	}
	if phaseUpdated.Phases[0].Priority != 3 {
		t.Fatalf("phase priority = %d, want %d", phaseUpdated.Phases[0].Priority, 3)
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

func TestDocEndpoints(t *testing.T) {
	srv, _ := newTestServer(t)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/docs", `{"plan_id":"web-ui","title":"Architecture Draft","content":"v1"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	created := decodeResponse[store.Doc](t, createRec)
	if created.ID != "architecture-draft" {
		t.Fatalf("created id = %q, want %q", created.ID, "architecture-draft")
	}
	if created.PlanID != "web-ui" {
		t.Fatalf("created plan_id = %q, want %q", created.PlanID, "web-ui")
	}

	listRec := performRequest(t, srv, http.MethodGet, "/api/docs?plan=web-ui")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listRec.Code, http.StatusOK)
	}
	listed := decodeResponse[[]store.Doc](t, listRec)
	if len(listed) != 1 {
		t.Fatalf("docs length = %d, want 1", len(listed))
	}

	getRec := performRequest(t, srv, http.MethodGet, "/api/docs/architecture-draft")
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/docs/architecture-draft", `{"content":"v2"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}
	updated := decodeResponse[store.Doc](t, updateRec)
	if updated.Content != "v2" {
		t.Fatalf("updated content = %q, want %q", updated.Content, "v2")
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

func TestDeleteDocEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	// Create doc first
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/docs", `{"title":"Temp Doc","content":"delete me"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}
	created := decodeResponse[store.Doc](t, createRec)

	// Delete it
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/docs/"+created.ID)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
	}

	// Verify it's gone
	getRec := performRequest(t, srv, http.MethodGet, "/api/docs/"+created.ID)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want %d", getRec.Code, http.StatusNotFound)
	}

	// Delete non-existent
	notFoundRec := performRequest(t, srv, http.MethodDelete, "/api/docs/no-such-doc")
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
	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"test-prof","agent":"claude","model":"sonnet"}`)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createRec.Code, http.StatusCreated)
	}

	// Verify it appears in list
	listRec = performRequest(t, srv, http.MethodGet, "/api/config/profiles")
	if listRec.Code != http.StatusOK {
		t.Fatalf("list after create status = %d, want %d", listRec.Code, http.StatusOK)
	}

	// Update profile
	updateRec := performJSONRequest(t, srv, http.MethodPut, "/api/config/profiles/test-prof", `{"name":"test-prof","agent":"claude","model":"opus"}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", updateRec.Code, http.StatusOK)
	}

	// Delete profile
	deleteRec := performRequest(t, srv, http.MethodDelete, "/api/config/profiles/test-prof")
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", deleteRec.Code, http.StatusOK)
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

func TestConfigLoopDefUpdateClearsOptionalStepFields(t *testing.T) {
	srv, _ := newTestServer(t)

	performJSONRequest(t, srv, http.MethodPost, "/api/config/profiles", `{"name":"loop-prof","agent":"claude"}`)

	createRec := performJSONRequest(t, srv, http.MethodPost, "/api/config/loops", `{
		"name":"test-loop",
		"steps":[{"profile":"loop-prof","turns":1,"team":"my-team","instructions":"hello"}]
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
	srv, _ := newTestServer(t)

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

	// Invalid phase status
	performJSONRequest(t, srv, http.MethodPost, "/api/plans", `{"id":"phase-test","title":"Phase Test","phases":[{"id":"p1","title":"P1","status":"not_started"}]}`)
	rec = performJSONRequest(t, srv, http.MethodPut, "/api/plans/phase-test/phases/p1", `{"status":"wrong"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid phase status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDocValidation(t *testing.T) {
	srv, _ := newTestServer(t)

	// Missing title
	rec := performJSONRequest(t, srv, http.MethodPost, "/api/docs", `{"content":"no title"}`)
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
