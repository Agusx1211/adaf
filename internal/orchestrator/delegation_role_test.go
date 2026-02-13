package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestSpawn_RequiresRoleWhenDelegationOptionIsAmbiguous(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "worker", Agent: "codex"},
		},
	}
	o := New(s, cfg, repo)

	_, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  1,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "do work",
		Delegation: &config.DelegationConfig{
			Profiles: []config.DelegationProfile{
				{Name: "worker", Role: config.RoleJunior},
				{Name: "worker", Role: config.RoleSenior},
			},
		},
	})
	if err == nil {
		t.Fatalf("Spawn() error = nil, want ambiguity error")
	}
	if !strings.Contains(err.Error(), "multiple roles") {
		t.Fatalf("Spawn() error = %q, want role ambiguity hint", err)
	}

	spawns, listErr := s.ListSpawns()
	if listErr != nil {
		t.Fatalf("ListSpawns: %v", listErr)
	}
	if len(spawns) != 0 {
		t.Fatalf("expected no spawn records on validation failure, got %d", len(spawns))
	}
}

func TestSpawn_RequiresDelegationRules(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "worker", Agent: "codex"},
		},
	}
	o := New(s, cfg, repo)

	_, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  1,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "do work",
		Delegation:    nil,
	})
	if err == nil {
		t.Fatalf("Spawn() error = nil, want missing delegation error")
	}
	if !strings.Contains(err.Error(), "explicit delegation rules") {
		t.Fatalf("Spawn() error = %q, want explicit delegation rules error", err)
	}
}

func TestSpawn_UsesResolvedRoleAndDelegationMetadata(t *testing.T) {
	repo := initGitRepo(t)
	s := newTestStore(t, repo)
	cfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "parent", Agent: "codex"},
			{Name: "worker", Agent: "missing-agent"},
		},
	}
	o := New(s, cfg, repo)

	spawnID, err := o.Spawn(context.Background(), SpawnRequest{
		ParentTurnID:  7,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		ChildRole:     config.RoleSenior,
		Task:          "test",
		Delegation: &config.DelegationConfig{
			Profiles: []config.DelegationProfile{
				{
					Name:    "worker",
					Role:    config.RoleJunior,
					Speed:   "fast",
					Handoff: false,
				},
				{
					Name:    "worker",
					Role:    config.RoleSenior,
					Speed:   "slow",
					Handoff: true,
				},
			},
		},
	})
	if err == nil {
		t.Fatalf("Spawn() error = nil, want missing-agent failure")
	}
	if spawnID == 0 {
		t.Fatalf("spawnID = 0, want non-zero")
	}

	rec, getErr := s.GetSpawn(spawnID)
	if getErr != nil {
		t.Fatalf("GetSpawn(%d): %v", spawnID, getErr)
	}
	if rec == nil {
		t.Fatalf("GetSpawn(%d) = nil", spawnID)
	}
	if rec.ChildRole != config.RoleSenior {
		t.Fatalf("ChildRole = %q, want %q", rec.ChildRole, config.RoleSenior)
	}
	if rec.Speed != "slow" {
		t.Fatalf("Speed = %q, want %q", rec.Speed, "slow")
	}
	if !rec.Handoff {
		t.Fatalf("Handoff = false, want true")
	}
}
