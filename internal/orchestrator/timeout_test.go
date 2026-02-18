package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestSpawn_TimedOutChildStopsAndReportsResumeGuidance(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)

	oldTimeoutUnit := childTimeoutUnit
	childTimeoutUnit = 25 * time.Millisecond
	defer func() {
		childTimeoutUnit = oldTimeoutUnit
	}()

	cmdPath := filepath.Join(t.TempDir(), "slow-generic.sh")
	script := "#!/usr/bin/env bash\nsleep 5\necho should-not-complete\n"
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

	start := time.Now()
	spawnID, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  101,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "long running task",
		Delegation: &config.DelegationConfig{
			Profiles: []config.DelegationProfile{{
				Name:           "worker",
				TimeoutMinutes: 1,
			}},
		},
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v, want nil", err)
	}
	if spawnID <= 0 {
		t.Fatalf("spawnID = %d, want > 0", spawnID)
	}

	got := o.WaitOne(spawnID)
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("spawn timeout path took too long: %v", elapsed)
	}

	if got.Status != store.SpawnStatusFailed {
		t.Fatalf("status = %q, want %q", got.Status, store.SpawnStatusFailed)
	}

	wantTimeout := "timed out after 1 minute"
	if !strings.Contains(strings.ToLower(got.Result), wantTimeout) {
		t.Fatalf("result = %q, want to contain %q", got.Result, wantTimeout)
	}
	if !strings.Contains(strings.ToLower(got.Summary), wantTimeout) {
		t.Fatalf("summary = %q, want to contain %q", got.Summary, wantTimeout)
	}
	if !strings.Contains(got.Summary, "verify it is making concrete progress") {
		t.Fatalf("summary = %q, want progress-check guidance", got.Summary)
	}
	if !strings.Contains(got.Summary, "adaf spawn --from-spawn "+strconv.Itoa(spawnID)+" --profile worker") {
		t.Fatalf("summary = %q, want resume guidance with spawn id", got.Summary)
	}
}
