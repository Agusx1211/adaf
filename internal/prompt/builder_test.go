package prompt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestBuild_SubAgentIncludesCommitRule(t *testing.T) {
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

	wantLine := "As a sub-agent, if you modify files you MUST create a git commit before finishing your turn."
	if !strings.Contains(got, wantLine) {
		t.Fatalf("missing sub-agent commit rule\nwant substring: %q\nprompt:\n%s", wantLine, got)
	}
}

func TestBuild_MainAgentDoesNotIncludeSubAgentCommitRule(t *testing.T) {
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

	wantLine := "As a sub-agent, if you modify files you MUST create a git commit before finishing your turn."
	if strings.Contains(got, wantLine) {
		t.Fatalf("unexpected sub-agent commit rule in main-agent prompt\nprompt:\n%s", got)
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

	if !strings.Contains(got, "`adaf parent-ask \"question\"`") {
		t.Fatalf("missing parent-ask guidance in sub-agent prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "`adaf parent-notify \"status update\"`") {
		t.Fatalf("missing parent-notify guidance in sub-agent prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "`adaf spawn-read-messages`") {
		t.Fatalf("missing spawn-read-messages guidance in sub-agent prompt\nprompt:\n%s", got)
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
	if strings.Contains(got, "`adaf parent-notify \"status update\"`") {
		t.Fatalf("unexpected parent-notify guidance in main-agent prompt\nprompt:\n%s", got)
	}
	if strings.Contains(got, "`adaf spawn-read-messages`") {
		t.Fatalf("unexpected spawn-read-messages guidance in main-agent prompt\nprompt:\n%s", got)
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
