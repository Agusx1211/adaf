package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestSpawn_AgentLookupFailureReleasesSlotsAndCleansWorktree(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "broken", Agent: "missing-agent"},
		},
	}
	o := New(s, cfg, repo)

	req := SpawnRequest{
		ParentTurnID:  62,
		ParentProfile: "parent",
		ChildProfile:  "broken",
		Task:          "test",
		ReadOnly:      false,
		Delegation: &config.DelegationConfig{
			Profiles: []config.DelegationProfile{{Name: "broken"}},
		},
	}

	spawnID, err := o.Spawn(context.Background(), req)
	if err == nil {
		t.Fatalf("Spawn error = nil, want error")
	}
	if !strings.Contains(err.Error(), "agent") {
		t.Fatalf("Spawn error = %q, want agent lookup failure", err)
	}
	if spawnID == 0 {
		t.Fatalf("spawnID = 0, want non-zero")
	}

	if got := o.running["parent"]; got != 0 {
		t.Fatalf("running[parent] = %d, want 0", got)
	}
	if got := o.instances["broken"]; got != 0 {
		t.Fatalf("instances[broken] = %d, want 0", got)
	}

	rec, getErr := s.GetSpawn(spawnID)
	if getErr != nil {
		t.Fatalf("GetSpawn(%d): %v", spawnID, getErr)
	}
	if rec == nil {
		t.Fatalf("GetSpawn(%d) = nil", spawnID)
	}
	if rec.Status != "failed" {
		t.Fatalf("spawn status = %q, want failed", rec.Status)
	}
	if rec.Branch == "" {
		t.Fatalf("spawn branch is empty, want branch name")
	}
	if rec.WorktreePath == "" {
		t.Fatalf("spawn worktree path is empty, want populated path")
	}

	if _, statErr := os.Stat(rec.WorktreePath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree path %q should be removed, stat err=%v", rec.WorktreePath, statErr)
	}
	if branchExists(repo, rec.Branch) {
		t.Fatalf("branch %q should be removed", rec.Branch)
	}
}

func TestSpawn_FailedChildSurfacesCrashDetailsForParent(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "broken", Agent: "generic"},
		},
	}
	o := New(s, cfg, repo)

	spawnID, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  73,
		ParentProfile: "parent",
		ChildProfile:  "broken",
		Task:          "implement feature",
		ReadOnly:      false,
		Delegation: &config.DelegationConfig{
			Profiles: []config.DelegationProfile{{Name: "broken"}},
		},
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v, want nil", err)
	}
	if spawnID <= 0 {
		t.Fatalf("spawnID = %d, want > 0", spawnID)
	}

	got := o.WaitOne(spawnID)
	if got.Status != "failed" {
		t.Fatalf("WaitOne(%d) status = %q, want failed", spawnID, got.Status)
	}
	if !strings.Contains(got.Result, "no command configured") {
		t.Fatalf("WaitOne(%d) result = %q, want launch error details", spawnID, got.Result)
	}
	if !strings.Contains(strings.ToLower(got.Summary), "sub-agent crashed") {
		t.Fatalf("WaitOne(%d) summary = %q, want crash note", spawnID, got.Summary)
	}
	if !strings.Contains(strings.ToLower(got.Summary), "may have finished some work") {
		t.Fatalf("WaitOne(%d) summary = %q, want partial-work warning", spawnID, got.Summary)
	}
	if !strings.Contains(got.Summary, "no command configured") {
		t.Fatalf("WaitOne(%d) summary = %q, want crash error details", spawnID, got.Summary)
	}
}

func TestSpawn_NonZeroExitMarksSpawnFailed(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)

	cmdPath := filepath.Join(t.TempDir(), "generic-exit-1.sh")
	script := "#!/usr/bin/env bash\n" +
		"echo \"[API Error: No capacity available for model gemini-3-flash-preview on the server]\"\n" +
		"echo \"Attempt 1 failed with status 429\" >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile(%q): %v", cmdPath, err)
	}

	if err := agent.SaveAgentsConfig(&agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: cmdPath},
		},
	}); err != nil {
		t.Fatalf("SaveAgentsConfig(): %v", err)
	}

	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "generic"},
			{Name: "worker", Agent: "generic"},
		},
	}
	o := New(s, cfg, repo)

	spawnID, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  174,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "simulate exhausted model capacity",
		ReadOnly:      false,
		Delegation: &config.DelegationConfig{
			Profiles: []config.DelegationProfile{{Name: "worker"}},
		},
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v, want nil", err)
	}
	if spawnID <= 0 {
		t.Fatalf("spawnID = %d, want > 0", spawnID)
	}

	got := o.WaitOne(spawnID)
	if got.Status != store.SpawnStatusFailed {
		t.Fatalf("WaitOne(%d) status = %q, want %q", spawnID, got.Status, store.SpawnStatusFailed)
	}
	if got.ExitCode != 1 {
		t.Fatalf("WaitOne(%d) exitCode = %d, want 1", spawnID, got.ExitCode)
	}
	if !strings.Contains(strings.ToLower(got.Summary), "sub-agent crashed") {
		t.Fatalf("WaitOne(%d) summary = %q, want crash note", spawnID, got.Summary)
	}
}
