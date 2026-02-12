package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreInit(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if s.Exists() {
		t.Fatal("store should not exist before init")
	}

	err = s.Init(ProjectConfig{
		Name:     "test-project",
		RepoPath: "/tmp/test-repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !s.Exists() {
		t.Fatal("store should exist after init")
	}

	// Check directories were created
	for _, sub := range []string{"logs", "records", "plans", "docs", "issues", "decisions"} {
		path := filepath.Join(dir, AdafDir, sub)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected directory %s to exist", sub)
		}
	}
}

func TestProjectConfig(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	cfg, err := s.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "test" {
		t.Errorf("expected name 'test', got %q", cfg.Name)
	}

	cfg.Name = "updated"
	if err := s.SaveProject(cfg); err != nil {
		t.Fatal(err)
	}

	cfg2, _ := s.LoadProject()
	if cfg2.Name != "updated" {
		t.Errorf("expected name 'updated', got %q", cfg2.Name)
	}
}

func TestIssues(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	// Create issues
	i1 := &Issue{Title: "Bug 1", Description: "First bug", Priority: "high"}
	if err := s.CreateIssue(i1); err != nil {
		t.Fatal(err)
	}
	if i1.ID != 1 {
		t.Errorf("expected ID 1, got %d", i1.ID)
	}

	i2 := &Issue{Title: "Bug 2", Description: "Second bug", Priority: "low"}
	if err := s.CreateIssue(i2); err != nil {
		t.Fatal(err)
	}
	if i2.ID != 2 {
		t.Errorf("expected ID 2, got %d", i2.ID)
	}

	// List
	issues, err := s.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}

	// Get
	got, err := s.GetIssue(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Bug 1" {
		t.Errorf("expected 'Bug 1', got %q", got.Title)
	}

	// Update
	got.Status = "resolved"
	if err := s.UpdateIssue(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetIssue(1)
	if got2.Status != "resolved" {
		t.Errorf("expected 'resolved', got %q", got2.Status)
	}
}

func TestSessionLogs(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	log1 := &SessionLog{Agent: "claude", Objective: "Fix build"}
	if err := s.CreateLog(log1); err != nil {
		t.Fatal(err)
	}

	log2 := &SessionLog{Agent: "codex", Objective: "Add tests"}
	if err := s.CreateLog(log2); err != nil {
		t.Fatal(err)
	}

	logs, _ := s.ListLogs()
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}

	latest, _ := s.LatestLog()
	if latest.Agent != "codex" {
		t.Errorf("expected latest agent 'codex', got %q", latest.Agent)
	}
}

func TestPlan(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	plan, _ := s.LoadPlan()
	if len(plan.Phases) != 0 {
		t.Error("expected empty plan")
	}

	plan.Title = "Master Plan"
	plan.Phases = []PlanPhase{
		{ID: "p1", Title: "Phase 1", Status: "complete"},
		{ID: "p2", Title: "Phase 2", Status: "in_progress"},
	}
	if err := s.SavePlan(plan); err != nil {
		t.Fatal(err)
	}

	plan2, _ := s.LoadPlan()
	if plan2.Title != "Master Plan" {
		t.Errorf("expected 'Master Plan', got %q", plan2.Title)
	}
	if len(plan2.Phases) != 2 {
		t.Errorf("expected 2 phases, got %d", len(plan2.Phases))
	}
}

func TestPlansMultiAPI(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	auth := &Plan{
		ID:          "auth-system",
		Title:       "Auth System",
		Description: "Authentication and authorization",
		Phases: []PlanPhase{
			{ID: "p1", Title: "JWT", Status: "not_started"},
		},
	}
	if err := s.CreatePlan(auth); err != nil {
		t.Fatalf("CreatePlan(auth): %v", err)
	}

	data := &Plan{
		ID:    "data-layer",
		Title: "Data Layer",
	}
	if err := s.CreatePlan(data); err != nil {
		t.Fatalf("CreatePlan(data): %v", err)
	}

	plans, err := s.ListPlans()
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}

	if err := s.SetActivePlan("auth-system"); err != nil {
		t.Fatalf("SetActivePlan(auth-system): %v", err)
	}
	active, err := s.ActivePlan()
	if err != nil {
		t.Fatalf("ActivePlan: %v", err)
	}
	if active == nil || active.ID != "auth-system" {
		t.Fatalf("expected active plan auth-system, got %#v", active)
	}

	authLoaded, err := s.GetPlan("auth-system")
	if err != nil {
		t.Fatalf("GetPlan(auth-system): %v", err)
	}
	authLoaded.Status = "done"
	if err := s.UpdatePlan(authLoaded); err != nil {
		t.Fatalf("UpdatePlan(auth-system): %v", err)
	}

	if err := s.DeletePlan("auth-system"); err != nil {
		t.Fatalf("DeletePlan(auth-system): %v", err)
	}
	deleted, err := s.GetPlan("auth-system")
	if err != nil {
		t.Fatalf("GetPlan(auth-system) after delete: %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected deleted plan to be nil")
	}
}

func TestLegacyPlanMigration(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	if err := s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"}); err != nil {
		t.Fatal(err)
	}

	legacyPath := filepath.Join(dir, AdafDir, "plan.json")
	legacy := `{"title":"Legacy Plan","description":"old","phases":[{"id":"p1","title":"phase","status":"not_started","priority":1}]}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	if err := s.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy plan to be removed, stat err=%v", err)
	}

	plan, err := s.GetPlan("default")
	if err != nil {
		t.Fatalf("GetPlan(default): %v", err)
	}
	if plan == nil {
		t.Fatal("expected migrated default plan")
	}
	if plan.Title != "Legacy Plan" {
		t.Fatalf("expected migrated title Legacy Plan, got %q", plan.Title)
	}
	if plan.Status != "active" {
		t.Fatalf("expected migrated status active, got %q", plan.Status)
	}

	project, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if project.ActivePlanID != "default" {
		t.Fatalf("expected active plan default, got %q", project.ActivePlanID)
	}
}

func TestIssuePlanFiltering(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	if err := s.CreateIssue(&Issue{Title: "shared", Priority: "medium"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateIssue(&Issue{Title: "auth", Priority: "high", PlanID: "auth-system"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateIssue(&Issue{Title: "data", Priority: "low", PlanID: "data-layer"}); err != nil {
		t.Fatal(err)
	}

	shared, err := s.ListSharedIssues()
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 1 || shared[0].Title != "shared" {
		t.Fatalf("unexpected shared issues: %#v", shared)
	}

	authIssues, err := s.ListIssuesForPlan("auth-system")
	if err != nil {
		t.Fatal(err)
	}
	if len(authIssues) != 2 {
		t.Fatalf("expected 2 auth-visible issues (shared+auth), got %d", len(authIssues))
	}
}

func TestDocPlanFiltering(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	if err := s.CreateDoc(&Doc{ID: "shared", Title: "Shared", Content: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDoc(&Doc{ID: "auth", Title: "Auth", Content: "a", PlanID: "auth-system"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDoc(&Doc{ID: "data", Title: "Data", Content: "d", PlanID: "data-layer"}); err != nil {
		t.Fatal(err)
	}

	shared, err := s.ListSharedDocs()
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 1 || shared[0].ID != "shared" {
		t.Fatalf("unexpected shared docs: %#v", shared)
	}

	authDocs, err := s.ListDocsForPlan("auth-system")
	if err != nil {
		t.Fatal(err)
	}
	if len(authDocs) != 2 {
		t.Fatalf("expected 2 auth-visible docs (shared+auth), got %d", len(authDocs))
	}
}

func TestDocs(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	doc := &Doc{ID: "arch", Title: "Architecture", Content: "# Architecture\nOverview..."}
	if err := s.CreateDoc(doc); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetDoc("arch")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Architecture" {
		t.Errorf("expected 'Architecture', got %q", got.Title)
	}

	got.Content = "# Updated\nNew content"
	if err := s.UpdateDoc(got); err != nil {
		t.Fatal(err)
	}

	docs, _ := s.ListDocs()
	if len(docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(docs))
	}
}

func TestRecordings(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	if err := s.AppendRecordingEvent(1, RecordingEvent{Type: "stdout", Data: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendRecordingEvent(1, RecordingEvent{Type: "stderr", Data: "warning"}); err != nil {
		t.Fatal(err)
	}

	rec := &SessionRecording{
		SessionID: 1,
		Agent:     "claude",
		Events: []RecordingEvent{
			{Type: "stdout", Data: "hello"},
			{Type: "stderr", Data: "warning"},
		},
	}
	if err := s.SaveRecording(rec); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.LoadRecording(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(loaded.Events))
	}
}

func TestGetDecision(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	dec := &Decision{
		Title:     "Use X",
		Context:   "Need a choice",
		Decision:  "Use X for now",
		Rationale: "Simpler",
	}
	if err := s.CreateDecision(dec); err != nil {
		t.Fatalf("CreateDecision() error = %v", err)
	}

	tests := []struct {
		name      string
		id        int
		wantErr   bool
		wantTitle string
	}{
		{
			name:      "existing decision",
			id:        dec.ID,
			wantErr:   false,
			wantTitle: "Use X",
		},
		{
			name:    "missing decision",
			id:      dec.ID + 1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.GetDecision(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetDecision(%d) error = nil, want error", tt.id)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetDecision(%d) error = %v", tt.id, err)
			}
			if got.Title != tt.wantTitle {
				t.Fatalf("GetDecision(%d) title = %q, want %q", tt.id, got.Title, tt.wantTitle)
			}
		})
	}
}
