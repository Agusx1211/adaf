package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestWiki(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	entry := &WikiEntry{
		ID:        "arch",
		Title:     "Architecture",
		Content:   "# Arch\nStuff here.",
		CreatedBy: "lead-agent",
		UpdatedBy: "lead-agent",
	}
	if err := s.CreateWikiEntry(entry); err != nil {
		t.Fatal(err)
	}

	wiki, _ := s.ListWiki()
	if len(wiki) != 1 {
		t.Errorf("expected 1 wiki entry, got %d", len(wiki))
	}

	got, err := s.GetWikiEntry("arch")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Architecture" {
		t.Errorf("title = %q, want %q", got.Title, "Architecture")
	}
	if got.CreatedBy != "lead-agent" {
		t.Errorf("created_by = %q, want %q", got.CreatedBy, "lead-agent")
	}
	if got.UpdatedBy != "lead-agent" {
		t.Errorf("updated_by = %q, want %q", got.UpdatedBy, "lead-agent")
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if len(got.History) != 1 {
		t.Errorf("history length = %d, want 1", len(got.History))
	}

	got.Content = "# Arch\nUpdated."
	got.UpdatedBy = "worker-agent"
	if err := s.UpdateWikiEntry(got); err != nil {
		t.Fatalf("UpdateWikiEntry: %v", err)
	}

	updated, err := s.GetWikiEntry("arch")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 {
		t.Errorf("updated version = %d, want 2", updated.Version)
	}
	if updated.UpdatedBy != "worker-agent" {
		t.Errorf("updated_by = %q, want %q", updated.UpdatedBy, "worker-agent")
	}
	if len(updated.History) != 2 {
		t.Errorf("history length = %d, want 2", len(updated.History))
	}
	if updated.History[1].Action != "update" {
		t.Errorf("history action = %q, want %q", updated.History[1].Action, "update")
	}
	if updated.History[1].By != "worker-agent" {
		t.Errorf("history by = %q, want %q", updated.History[1].By, "worker-agent")
	}
}

func TestSearchWiki(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	entries := []*WikiEntry{
		{ID: "release-runbook", Title: "Release Runbook", Content: "Deploy process and rollback guidance."},
		{ID: "agent-handoff", Title: "Agent Handoff", Content: "How workers should pass context."},
		{ID: "database-notes", Title: "Database Notes", Content: "Schema and indexes."},
	}
	for _, entry := range entries {
		entry.CreatedBy = "seed"
		entry.UpdatedBy = "seed"
		if err := s.CreateWikiEntry(entry); err != nil {
			t.Fatalf("CreateWikiEntry(%s): %v", entry.ID, err)
		}
	}

	results, err := s.SearchWiki("release", 5)
	if err != nil {
		t.Fatalf("SearchWiki: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty search results")
	}
	if results[0].ID != "release-runbook" {
		t.Fatalf("top result = %q, want %q", results[0].ID, "release-runbook")
	}

	fuzzyResults, err := s.SearchWiki("rlsrbk", 5)
	if err != nil {
		t.Fatalf("SearchWiki fuzzy: %v", err)
	}
	if len(fuzzyResults) == 0 {
		t.Fatal("expected fuzzy search results")
	}
	if fuzzyResults[0].ID != "release-runbook" {
		t.Fatalf("top fuzzy result = %q, want %q", fuzzyResults[0].ID, "release-runbook")
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

func TestIsTurnFrozen(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name string
		turn *Turn
		want bool
	}{
		{
			name: "nil",
			turn: nil,
			want: false,
		},
		{
			name: "active",
			turn: &Turn{BuildState: ""},
			want: false,
		},
		{
			name: "waiting",
			turn: &Turn{BuildState: "waiting_for_spawns"},
			want: false,
		},
		{
			name: "success",
			turn: &Turn{BuildState: "success"},
			want: true,
		},
		{
			name: "exit code",
			turn: &Turn{BuildState: "exit_code_1"},
			want: true,
		},
		{
			name: "finalized timestamp",
			turn: &Turn{FinalizedAt: now, BuildState: ""},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTurnFrozen(tt.turn); got != tt.want {
				t.Fatalf("IsTurnFrozen() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlan(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	p := &Plan{
		ID:          "plan1",
		Title:       "Test Plan",
		Description: "High-level rationale and scope",
		Status:      "active",
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
	if loaded.Description != "High-level rationale and scope" {
		t.Errorf("plan description = %q, want %q", loaded.Description, "High-level rationale and scope")
	}
}

func TestIssueLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	issue := &Issue{Title: "bug", Status: "open", Priority: "high"}
	s.CreateIssue(issue)

	issue.Status = IssueStatusClosed
	issue.UpdatedBy = "agent-x"
	if err := s.UpdateIssue(issue); err != nil {
		t.Fatal(err)
	}

	got, _ := s.GetIssue(issue.ID)
	if got.Status != IssueStatusClosed {
		t.Errorf("status = %q, want %q", got.Status, IssueStatusClosed)
	}
	if got.UpdatedBy != "agent-x" {
		t.Errorf("updated_by = %q, want %q", got.UpdatedBy, "agent-x")
	}
	if len(got.History) < 2 {
		t.Fatalf("history length = %d, want >= 2", len(got.History))
	}
}

func TestIssueCommentsAndHistory(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	issue := &Issue{
		Title:     "Threaded issue",
		Status:    IssueStatusOpen,
		Priority:  "medium",
		CreatedBy: "lead-agent",
		UpdatedBy: "lead-agent",
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	updated, err := s.AddIssueComment(issue.ID, "Investigating root cause", "worker-agent")
	if err != nil {
		t.Fatalf("AddIssueComment: %v", err)
	}
	if len(updated.Comments) != 1 {
		t.Fatalf("comment length = %d, want 1", len(updated.Comments))
	}
	if updated.Comments[0].By != "worker-agent" {
		t.Fatalf("comment by = %q, want %q", updated.Comments[0].By, "worker-agent")
	}
	if len(updated.History) < 2 {
		t.Fatalf("history length = %d, want >= 2", len(updated.History))
	}
	last := updated.History[len(updated.History)-1]
	if last.Type != "commented" {
		t.Fatalf("last history type = %q, want %q", last.Type, "commented")
	}
}

func TestValidateIssueDependencies(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	s.Init(ProjectConfig{Name: "test", RepoPath: "/tmp"})

	iss1 := &Issue{Title: "Issue 1", Status: "open", Priority: "high"}
	iss2 := &Issue{Title: "Issue 2", Status: "open", Priority: "medium"}
	if err := s.CreateIssue(iss1); err != nil {
		t.Fatalf("CreateIssue(iss1): %v", err)
	}
	if err := s.CreateIssue(iss2); err != nil {
		t.Fatalf("CreateIssue(iss2): %v", err)
	}

	deps, err := s.ValidateIssueDependencies(iss2.ID, []int{iss1.ID, iss1.ID})
	if err != nil {
		t.Fatalf("ValidateIssueDependencies(no cycle): %v", err)
	}
	if len(deps) != 1 || deps[0] != iss1.ID {
		t.Fatalf("normalized deps = %v, want [%d]", deps, iss1.ID)
	}
	iss2.DependsOn = deps
	if err := s.UpdateIssue(iss2); err != nil {
		t.Fatalf("UpdateIssue(iss2): %v", err)
	}

	if _, err := s.ValidateIssueDependencies(iss1.ID, []int{iss2.ID}); err == nil {
		t.Fatalf("expected cycle validation error")
	}
	if _, err := s.ValidateIssueDependencies(iss1.ID, []int{999}); err == nil {
		t.Fatalf("expected unknown dependency validation error")
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
