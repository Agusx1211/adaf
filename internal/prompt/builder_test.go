package prompt

import (
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
