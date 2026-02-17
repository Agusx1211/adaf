package cli

import (
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestLoopProfilesSnapshot_CollectsTeamDelegationProfiles(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "manager", Agent: "codex"},
			{Name: "developer", Agent: "codex"},
			{Name: "scout", Agent: "codex"},
		},
		Teams: []config.Team{
			{
				Name: "dev-team",
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{Name: "developer", Role: config.RoleDeveloper},
						{Name: "scout", Role: config.RoleScout},
					},
				},
			},
		},
	}
	loopDef := &config.LoopDef{
		Name: "nested",
		Steps: []config.LoopStep{
			{
				Profile: "manager",
				Team:    "dev-team",
			},
		},
	}

	profiles, err := loopProfilesSnapshot(globalCfg, loopDef)
	if err != nil {
		t.Fatalf("loopProfilesSnapshot() error = %v", err)
	}

	got := make(map[string]struct{}, len(profiles))
	for _, p := range profiles {
		got[p.Name] = struct{}{}
	}
	for _, name := range []string{"manager", "developer", "scout"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("profile %q missing from snapshot", name)
		}
	}
}

func TestLoopProfilesSnapshot_ErrorsOnMissingTeamProfile(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "manager", Agent: "codex"},
		},
		Teams: []config.Team{
			{
				Name: "bad-team",
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{Name: "missing"},
					},
				},
			},
		},
	}
	loopDef := &config.LoopDef{
		Name: "nested",
		Steps: []config.LoopStep{
			{
				Profile: "manager",
				Team:    "bad-team",
			},
		},
	}

	if _, err := loopProfilesSnapshot(globalCfg, loopDef); err == nil {
		t.Fatalf("loopProfilesSnapshot() error = nil, want missing profile error")
	}
}
