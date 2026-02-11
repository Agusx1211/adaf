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
	for _, sub := range []string{"logs", "recordings", "docs", "issues", "decisions"} {
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
