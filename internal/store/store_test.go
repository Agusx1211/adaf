package store

import (
	"testing"
)

func TestInit(t *testing.T) {
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

func TestDecisions(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	d := &Decision{Title: "use postgres", Decision: "pg", Rationale: "mature"}
	if err := s.CreateDecision(d); err != nil {
		t.Fatal(err)
	}
	if d.ID == 0 {
		t.Error("expected non-zero ID")
	}

	decisions, _ := s.ListDecisions()
	if len(decisions) != 1 {
		t.Errorf("expected 1 decision, got %d", len(decisions))
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

func TestDecisionLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	d := &Decision{Title: "test", Decision: "yes", Rationale: "why not"}
	s.CreateDecision(d)

	got, err := s.GetDecision(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "test" {
		t.Errorf("title = %q, want %q", got.Title, "test")
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

func TestSupervisorNotes(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	note1 := &SupervisorNote{TurnID: 1, Author: "supervisor", Note: "build is green"}
	if err := s.CreateNote(note1); err != nil {
		t.Fatal(err)
	}
	note2 := &SupervisorNote{TurnID: 1, Author: "supervisor", Note: "check integration tests"}
	if err := s.CreateNote(note2); err != nil {
		t.Fatal(err)
	}
	note3 := &SupervisorNote{TurnID: 2, Author: "supervisor", Note: "different turn"}
	if err := s.CreateNote(note3); err != nil {
		t.Fatal(err)
	}

	notes, err := s.NotesByTurn(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 {
		t.Errorf("expected 2 notes for turn 1, got %d", len(notes))
	}

	notes2, _ := s.NotesByTurn(2)
	if len(notes2) != 1 {
		t.Errorf("expected 1 note for turn 2, got %d", len(notes2))
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

	// Send a message.
	msg := &SpawnMessage{
		SpawnID:   spawn.ID,
		Direction: "parent_to_child",
		Type:      "message",
		Content:   "hello child",
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

	// Unread.
	unread, err := s.UnreadMessages(spawn.ID, "child_to_parent")
	if err != nil {
		t.Fatal(err)
	}
	// The message was parent_to_child, so child_to_parent unread should be 0.
	if len(unread) != 0 {
		t.Errorf("expected 0 unread child_to_parent, got %d", len(unread))
	}

	// But unread from parent perspective should have 1.
	unreadP, err := s.UnreadMessages(spawn.ID, "parent_to_child")
	if err != nil {
		t.Fatal(err)
	}
	if len(unreadP) != 1 {
		t.Errorf("expected 1 unread parent_to_child, got %d", len(unreadP))
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
