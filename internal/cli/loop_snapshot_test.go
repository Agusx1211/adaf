package cli

import (
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestLoopProfilesSnapshot_CollectsNestedDelegationProfiles(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "manager", Agent: "codex"},
			{Name: "senior-dev", Agent: "codex"},
			{Name: "junior-dev", Agent: "codex"},
			{Name: "scout", Agent: "codex"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "nested",
		Steps: []config.LoopStep{
			{
				Profile: "manager",
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{
							Name: "senior-dev",
							Role: config.RoleSenior,
							Delegation: &config.DelegationConfig{
								Profiles: []config.DelegationProfile{
									{Name: "scout", Role: config.RoleJunior},
								},
							},
						},
						{Name: "junior-dev", Role: config.RoleJunior},
					},
				},
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
	for _, name := range []string{"manager", "senior-dev", "junior-dev", "scout"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("profile %q missing from snapshot", name)
		}
	}
}

func TestLoopProfilesSnapshot_ErrorsOnMissingNestedProfile(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "manager", Agent: "codex"},
		},
	}
	loopDef := &config.LoopDef{
		Name: "nested",
		Steps: []config.LoopStep{
			{
				Profile: "manager",
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{Name: "missing"},
					},
				},
			},
		},
	}

	if _, err := loopProfilesSnapshot(globalCfg, loopDef); err == nil {
		t.Fatalf("loopProfilesSnapshot() error = nil, want missing profile error")
	}
}
