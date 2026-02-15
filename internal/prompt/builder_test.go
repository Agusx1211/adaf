package prompt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestBuild_TopLevelIncludesAutonomyRule(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:   s,
		Project: project,
		Profile: profile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(got, "fully autonomous") {
		t.Fatalf("missing autonomy rule for top-level agent\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "no human in the loop") {
		t.Fatalf("missing no-human-in-the-loop rule for top-level agent\nprompt:\n%s", got)
	}
}

func TestBuild_SubAgentOmitsAutonomyRule(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:        s,
		Project:      project,
		Profile:      profile,
		ParentTurnID: 100,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(got, "no human in the loop") {
		t.Fatalf("sub-agent should not see autonomy rule (parent is their human)\nprompt:\n%s", got)
	}
}

func TestBuild_IncludesCommitOwnershipRule(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:   s,
		Project: project,
		Profile: profile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(got, "You own your repository") {
		t.Fatalf("missing commit ownership rule\nprompt:\n%s", got)
	}
}

func TestBuild_ReadOnlyOmitsCommitRule(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:    s,
		Project:  project,
		Profile:  profile,
		ReadOnly: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(got, "You own your repository") {
		t.Fatalf("read-only agent should not have commit ownership rule\nprompt:\n%s", got)
	}
}

func TestBuild_SubAgentIncludesParentCommunicationCommands(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:        s,
		Project:      project,
		Profile:      profile,
		ParentTurnID: 100,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(got, "You are a sub-agent working as a") {
		t.Fatalf("missing sub-agent intro in prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "`adaf parent-ask \"question\"`") {
		t.Fatalf("missing parent-ask guidance in sub-agent prompt\nprompt:\n%s", got)
	}
}

func TestBuild_MainAgentDoesNotIncludeParentCommunicationCommands(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:   s,
		Project: project,
		Profile: profile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if strings.Contains(got, "`adaf parent-ask \"question\"`") {
		t.Fatalf("unexpected parent-ask guidance in main-agent prompt\nprompt:\n%s", got)
	}
}

func TestBuild_RecentTurnsInjection(t *testing.T) {
	s, project := initPromptTestStore(t)

	// Create 7 turns so we can verify only the last 5 are shown.
	for i := 1; i <= 7; i++ {
		turn := &store.Turn{
			Agent:        "claude",
			Objective:    fmt.Sprintf("Objective for turn %d", i),
			WhatWasBuilt: fmt.Sprintf("Built thing %d", i),
			NextSteps:    fmt.Sprintf("Next steps %d", i),
			KnownIssues:  fmt.Sprintf("Issues %d", i),
			CurrentState: fmt.Sprintf("State %d", i),
			BuildState:   fmt.Sprintf("build-ok-%d", i),
		}
		if err := s.CreateTurn(turn); err != nil {
			t.Fatalf("CreateTurn %d: %v", i, err)
		}
	}

	profile := &config.Profile{
		Name:  "dev",
		Agent: "claude",
	}

	got, err := Build(BuildOpts{
		Store:   s,
		Project: project,
		Profile: profile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Should show "7 session logs total" with "5 most recent".
	if !strings.Contains(got, "7 session logs total") {
		t.Fatalf("missing total count\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "5 most recent") {
		t.Fatalf("missing '5 most recent'\nprompt:\n%s", got)
	}

	// Turns 1 and 2 should NOT appear (they're outside the window).
	if strings.Contains(got, "Objective for turn 1") {
		t.Fatalf("turn 1 should not be in recent turns\nprompt:\n%s", got)
	}
	if strings.Contains(got, "Objective for turn 2") {
		t.Fatalf("turn 2 should not be in recent turns\nprompt:\n%s", got)
	}

	// Turns 3-7 should appear.
	for i := 3; i <= 7; i++ {
		want := fmt.Sprintf("Objective for turn %d", i)
		if !strings.Contains(got, want) {
			t.Fatalf("missing turn %d objective\nwant: %q\nprompt:\n%s", i, want, got)
		}
	}

	// Only the latest (turn 7) should have full detail fields like BuildState.
	if !strings.Contains(got, "build-ok-7") {
		t.Fatalf("latest turn (7) missing build state\nprompt:\n%s", got)
	}
	// Older turns should NOT have build state.
	if strings.Contains(got, "build-ok-3") {
		t.Fatalf("older turn (3) should not have build state\nprompt:\n%s", got)
	}

	// No truncation: full text should be present.
	if !strings.Contains(got, "Objective for turn 7") {
		t.Fatalf("latest turn objective should not be truncated\nprompt:\n%s", got)
	}
}

func TestBuild_NoTurns(t *testing.T) {
	s, project := initPromptTestStore(t)

	profile := &config.Profile{
		Name:  "dev",
		Agent: "claude",
	}

	got, err := Build(BuildOpts{
		Store:   s,
		Project: project,
		Profile: profile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Should not contain session log section at all.
	if strings.Contains(got, "Recent Session Logs") {
		t.Fatalf("should not have session logs section with no turns\nprompt:\n%s", got)
	}
}

func TestBuild_FewTurns(t *testing.T) {
	s, project := initPromptTestStore(t)

	// Create only 2 turns (less than max of 5).
	for i := 1; i <= 2; i++ {
		turn := &store.Turn{
			Agent:     "codex",
			Objective: fmt.Sprintf("Objective %d", i),
		}
		if err := s.CreateTurn(turn); err != nil {
			t.Fatalf("CreateTurn %d: %v", i, err)
		}
	}

	profile := &config.Profile{
		Name:  "dev",
		Agent: "codex",
	}

	got, err := Build(BuildOpts{
		Store:   s,
		Project: project,
		Profile: profile,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Should show Recent Session Logs but NOT the "N total" line (since all are shown).
	if !strings.Contains(got, "Recent Session Logs") {
		t.Fatalf("missing session logs section\nprompt:\n%s", got)
	}
	if strings.Contains(got, "session logs total") {
		t.Fatalf("should not show total count when all turns fit\nprompt:\n%s", got)
	}

	// Both turns should appear.
	if !strings.Contains(got, "Objective 1") {
		t.Fatalf("missing turn 1\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Objective 2") {
		t.Fatalf("missing turn 2\nprompt:\n%s", got)
	}
}

func TestBuild_SubAgentSkipsSessionLogsAndIssues(t *testing.T) {
	s, project := initPromptTestStore(t)

	// Create turns and issues.
	for i := 1; i <= 3; i++ {
		turn := &store.Turn{
			Agent:     "claude",
			Objective: fmt.Sprintf("Turn objective %d", i),
		}
		if err := s.CreateTurn(turn); err != nil {
			t.Fatalf("CreateTurn %d: %v", i, err)
		}
	}
	if err := s.CreateIssue(&store.Issue{
		Title:    "Some open issue",
		Status:   "open",
		Priority: "high",
	}); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	profile := &config.Profile{
		Name:  "dev",
		Agent: "claude",
	}

	got, err := Build(BuildOpts{
		Store:        s,
		Project:      project,
		Profile:      profile,
		ParentTurnID: 100,
		Task:         "Fix the auth bug",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Sub-agent should NOT see session logs.
	if strings.Contains(got, "Recent Session Logs") {
		t.Fatalf("sub-agent should not see session logs\nprompt:\n%s", got)
	}
	if strings.Contains(got, "Turn objective") {
		t.Fatalf("sub-agent should not see turn objectives\nprompt:\n%s", got)
	}

	// Sub-agent should NOT see open issues.
	if strings.Contains(got, "Open Issues") {
		t.Fatalf("sub-agent should not see open issues\nprompt:\n%s", got)
	}
	if strings.Contains(got, "Some open issue") {
		t.Fatalf("sub-agent should not see issue content\nprompt:\n%s", got)
	}
}

func TestBuild_SubAgentShowsAssignedIssues(t *testing.T) {
	s, project := initPromptTestStore(t)

	issue1 := &store.Issue{Title: "Auth bug", Status: "open", Priority: "high", Description: "Login fails"}
	issue2 := &store.Issue{Title: "Perf issue", Status: "open", Priority: "medium", Description: "Slow query"}
	issue3 := &store.Issue{Title: "Unrelated", Status: "open", Priority: "low", Description: "Other thing"}
	if err := s.CreateIssue(issue1); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := s.CreateIssue(issue2); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if err := s.CreateIssue(issue3); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	profile := &config.Profile{
		Name:  "dev",
		Agent: "claude",
	}

	got, err := Build(BuildOpts{
		Store:        s,
		Project:      project,
		Profile:      profile,
		ParentTurnID: 100,
		Task:         "Fix assigned issues",
		IssueIDs:     []int{issue1.ID, issue2.ID},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(got, "Assigned Issues") {
		t.Fatalf("missing Assigned Issues section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Auth bug") {
		t.Fatalf("missing assigned issue 'Auth bug'\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Perf issue") {
		t.Fatalf("missing assigned issue 'Perf issue'\nprompt:\n%s", got)
	}
	if strings.Contains(got, "Unrelated") {
		t.Fatalf("should not include unassigned issue\nprompt:\n%s", got)
	}
}

func TestBuild_DelegationIncludesRunningSpawnStateAndCapacity(t *testing.T) {
	s, project := initPromptTestStore(t)

	running := &store.SpawnRecord{
		ParentTurnID:  77,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		ChildRole:     config.RoleDeveloper,
		Task:          "active work",
		Status:        "running",
	}
	if err := s.CreateSpawn(running); err != nil {
		t.Fatalf("CreateSpawn(running): %v", err)
	}
	completed := &store.SpawnRecord{
		ParentTurnID:  77,
		ParentProfile: "manager",
		ChildProfile:  "worker",
		Task:          "done work",
		Status:        "completed",
	}
	if err := s.CreateSpawn(completed); err != nil {
		t.Fatalf("CreateSpawn(completed): %v", err)
	}

	profile := &config.Profile{Name: "manager", Agent: "claude"}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "manager", Agent: "claude"},
			{Name: "worker", Agent: "codex", MaxInstances: 1},
		},
	}
	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker", Role: config.RoleDeveloper, MaxInstances: 1},
		},
	}

	got, err := Build(BuildOpts{
		Store:         s,
		Project:       project,
		Profile:       profile,
		GlobalCfg:     globalCfg,
		Delegation:    deleg,
		CurrentTurnID: 77,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if !strings.Contains(got, "Currently Running Spawns") {
		t.Fatalf("missing running spawns section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprintf("Spawn #%d", running.ID)) {
		t.Fatalf("missing running spawn id in prompt\nprompt:\n%s", got)
	}
	if strings.Contains(got, fmt.Sprintf("Spawn #%d", completed.ID)) {
		t.Fatalf("completed spawn should not appear in running section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "running=1/1") || !strings.Contains(got, "[at-cap]") {
		t.Fatalf("missing capacity/running indicator in prompt\nprompt:\n%s", got)
	}
}

func initPromptTestStore(t *testing.T) (*store.Store, *store.ProjectConfig) {
	t.Helper()
	dir := t.TempDir()

	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	cfg := store.ProjectConfig{
		Name:     "prompt-test",
		RepoPath: dir,
	}
	if err := s.Init(cfg); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	project, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	return s, project
}
