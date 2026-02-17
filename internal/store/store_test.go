package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := ProjectConfig{Name: "myapp", RepoPath: "/tmp/myapp"}
	if err := s.Init(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := s.LoadProject()
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "myapp" {
		t.Errorf("name = %q, want %q", got.Name, "myapp")
	}
	if s.ProjectID() == "" {
		t.Fatal("project id is empty")
	}
	if _, err := os.Stat(ProjectMarkerPath(dir)); err != nil {
		t.Fatalf("expected marker file, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), ".git")); err != nil {
		t.Fatalf("expected store git repo, stat err=%v", err)
	}
}

func TestNewMigratesOperationalDirsToLocalScope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	projectID, err := GenerateProjectID(repo)
	if err != nil {
		t.Fatalf("GenerateProjectID() error = %v", err)
	}
	if err := writeProjectMarker(repo, projectID); err != nil {
		t.Fatalf("writeProjectMarker() error = %v", err)
	}
	root := ProjectStoreDirForID(projectID)

	oldEvents := filepath.Join(root, "records", "1", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(oldEvents), 0755); err != nil {
		t.Fatalf("MkdirAll(old records) error = %v", err)
	}
	if err := os.WriteFile(oldEvents, []byte("{\"type\":\"stdout\",\"data\":\"hello\"}\n"), 0644); err != nil {
		t.Fatalf("WriteFile(old events) error = %v", err)
	}

	oldTurn := filepath.Join(root, "turns", "1.json")
	if err := os.MkdirAll(filepath.Dir(oldTurn), 0755); err != nil {
		t.Fatalf("MkdirAll(old turns) error = %v", err)
	}
	if err := os.WriteFile(oldTurn, []byte("{}"), 0644); err != nil {
		t.Fatalf("WriteFile(old turn) error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "stats", "loops"), 0755); err != nil {
		t.Fatalf("MkdirAll(old stats) error = %v", err)
	}

	s, err := New(repo)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	newEvents := filepath.Join(s.Root(), "local", "records", "1", "events.jsonl")
	if _, err := os.Stat(newEvents); err != nil {
		t.Fatalf("expected migrated events file at %q, stat err=%v", newEvents, err)
	}

	newTurn := filepath.Join(s.Root(), "local", "turns", "1.json")
	if _, err := os.Stat(newTurn); err != nil {
		t.Fatalf("expected migrated turn file at %q, stat err=%v", newTurn, err)
	}

	oldRecordsDir := filepath.Join(s.Root(), "records")
	if _, err := os.Stat(oldRecordsDir); !os.IsNotExist(err) {
		t.Fatalf("expected old records dir to be absent after migration, stat err=%v", err)
	}

	oldTurnsDir := filepath.Join(s.Root(), "turns")
	if _, err := os.Stat(oldTurnsDir); !os.IsNotExist(err) {
		t.Fatalf("expected old turns dir to be absent after migration, stat err=%v", err)
	}

	if err := s.migrateToLocalScope(); err != nil {
		t.Fatalf("second migrateToLocalScope() error = %v", err)
	}
	if _, err := os.Stat(newEvents); err != nil {
		t.Fatalf("expected migrated events file to remain after second migration, stat err=%v", err)
	}
}

func TestInitDoesNotCreateRetiredDirectories(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(ProjectConfig{Name: "myapp", RepoPath: dir}); err != nil {
		t.Fatal(err)
	}

	for _, retired := range []string{"decisions", "notes"} {
		path := filepath.Join(s.Root(), retired)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected retired dir %q to be absent", path)
		}
	}
}

func TestIssues(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	issue := &Issue{Title: "bug", Status: "open", Priority: "high"}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatal(err)
	}
	if issue.ID == 0 {
		t.Error("expected non-zero ID")
	}

	issues, _ := s.ListIssues()
	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
}

func TestDocs(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	doc := &Doc{ID: "arch", Title: "Architecture", Content: "# Arch\nStuff here."}
	if err := s.CreateDoc(doc); err != nil {
		t.Fatal(err)
	}

	docs, _ := s.ListDocs()
	if len(docs) != 1 {
		t.Errorf("expected 1 doc, got %d", len(docs))
	}

	got, err := s.GetDoc("arch")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Architecture" {
		t.Errorf("title = %q, want %q", got.Title, "Architecture")
	}
}

func TestPlans(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	plan := &Plan{ID: "p1", Title: "MVP", Status: "active"}
	if err := s.SavePlan(plan); err != nil {
		t.Fatal(err)
	}

	plans, _ := s.ListPlans()
	if len(plans) != 1 {
		t.Errorf("expected 1 plan, got %d", len(plans))
	}
}

func TestTurns(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	log1 := &Turn{Agent: "claude", Objective: "Fix build"}
	if err := s.CreateTurn(log1); err != nil {
		t.Fatal(err)
	}

	log2 := &Turn{Agent: "codex", Objective: "Add tests"}
	if err := s.CreateTurn(log2); err != nil {
		t.Fatal(err)
	}

	turns, _ := s.ListTurns()
	if len(turns) != 2 {
		t.Errorf("expected 2 turns, got %d", len(turns))
	}

	latest, _ := s.LatestTurn()
	if latest.Agent != "codex" {
		t.Errorf("expected latest agent 'codex', got %q", latest.Agent)
	}
}

func TestPlan(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	p := &Plan{
		ID:     "plan1",
		Title:  "Test Plan",
		Status: "active",
		Phases: []PlanPhase{
			{ID: "p1", Title: "Phase 1", Status: "not_started"},
			{ID: "p2", Title: "Phase 2", Status: "not_started", DependsOn: []string{"p1"}},
		},
	}
	if err := s.SavePlan(p); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.GetPlan("plan1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Test Plan" {
		t.Errorf("plan title = %q, want %q", loaded.Title, "Test Plan")
	}
	if len(loaded.Phases) != 2 {
		t.Errorf("phases = %d, want 2", len(loaded.Phases))
	}
}

func TestIssueLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	issue := &Issue{Title: "bug", Status: "open", Priority: "high"}
	s.CreateIssue(issue)

	issue.Status = "resolved"
	if err := s.UpdateIssue(issue); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetIssue(issue.ID)
	if got.Status != "resolved" {
		t.Errorf("status = %q, want %q", got.Status, "resolved")
	}
}

func TestSpawnOperations(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	rec := &SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "architect",
		ChildProfile:  "builder",
		Task:          "implement feature X",
		Status:        "running",
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatal(err)
	}
	if rec.ID == 0 {
		t.Error("expected non-zero spawn ID")
	}

	// Get
	got, err := s.GetSpawn(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Task != "implement feature X" {
		t.Errorf("task = %q", got.Task)
	}

	// Update
	got.Status = "completed"
	got.Result = "done"
	if err := s.UpdateSpawn(got); err != nil {
		t.Fatal(err)
	}

	updated, _ := s.GetSpawn(rec.ID)
	if updated.Status != "completed" {
		t.Errorf("status = %q, want completed", updated.Status)
	}

	// List
	spawns, _ := s.ListSpawns()
	if len(spawns) != 1 {
		t.Errorf("expected 1 spawn, got %d", len(spawns))
	}

	// ByParent
	byParent, _ := s.SpawnsByParent(1)
	if len(byParent) != 1 {
		t.Errorf("expected 1 spawn by parent, got %d", len(byParent))
	}
}

func TestRecording(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	// Append events first.
	if err := s.AppendRecordingEvent(1, RecordingEvent{Type: "stdout", Data: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendRecordingEvent(1, RecordingEvent{Type: "stderr", Data: "warning"}); err != nil {
		t.Fatal(err)
	}

	rec := &TurnRecording{
		TurnID: 1,
		Agent:  "claude",
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
	if loaded.Agent != "claude" {
		t.Errorf("agent = %q", loaded.Agent)
	}
	if len(loaded.Events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(loaded.Events))
	}
}

func TestSpawnMessages(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	// Create a spawn first.
	spawn := &SpawnRecord{ParentTurnID: 1, ParentProfile: "a", ChildProfile: "b", Task: "x", Status: "running"}
	s.CreateSpawn(spawn)

	// Create an ask message.
	msg := &SpawnMessage{
		SpawnID:   spawn.ID,
		Direction: "child_to_parent",
		Type:      "ask",
		Content:   "what should I do?",
	}
	if err := s.CreateMessage(msg); err != nil {
		t.Fatal(err)
	}
	if msg.ID == 0 {
		t.Error("expected non-zero message ID")
	}

	// List messages.
	msgs, err := s.ListMessages(spawn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}

	// PendingAsk should find the unanswered ask.
	pending, err := s.PendingAsk(spawn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil {
		t.Fatal("expected pending ask, got nil")
	}
	if pending.Content != "what should I do?" {
		t.Errorf("pending content = %q", pending.Content)
	}

	// Reply to the ask.
	reply := &SpawnMessage{
		SpawnID:   spawn.ID,
		Direction: "parent_to_child",
		Type:      "reply",
		Content:   "do X",
		ReplyToID: msg.ID,
	}
	if err := s.CreateMessage(reply); err != nil {
		t.Fatal(err)
	}

	// PendingAsk should now return nil.
	pending, err = s.PendingAsk(spawn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending != nil {
		t.Errorf("expected no pending ask after reply, got message #%d", pending.ID)
	}
}

func TestLoopRun(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	run := &LoopRun{
		LoopName: "dev",
		Status:   "running",
		Steps: []LoopRunStep{
			{Profile: "architect", Turns: 1},
			{Profile: "builder", Turns: 3},
		},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatal(err)
	}
	if run.ID == 0 {
		t.Error("expected non-zero run ID")
	}

	got, err := s.GetLoopRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LoopName != "dev" {
		t.Errorf("loop name = %q", got.LoopName)
	}

	// Update.
	got.Cycle = 1
	if err := s.UpdateLoopRun(got); err != nil {
		t.Fatal(err)
	}

	updated, _ := s.GetLoopRun(run.ID)
	if updated.Cycle != 1 {
		t.Errorf("cycle = %d, want 1", updated.Cycle)
	}
}
