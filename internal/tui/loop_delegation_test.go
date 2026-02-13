package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/config"
)

func TestDelegationConfigToNodesSupportsMultiRoleProfiles(t *testing.T) {
	cfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(cfg)

	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{
				Name:    "worker",
				Roles:   []string{config.RoleDeveloper, config.RoleUIDesigner},
				Speed:   "fast",
				Handoff: true,
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{
						{Name: "scouty", Role: config.RoleScout},
					},
				},
			},
		},
	}

	got := delegationConfigToNodes(deleg, cfg)
	if len(got) != 1 {
		t.Fatalf("len(nodes) = %d, want 1", len(got))
	}
	node := got[0]
	if node.Profile != "worker" {
		t.Fatalf("node profile = %q, want worker", node.Profile)
	}
	if !equalStrings(node.Roles, []string{config.RoleDeveloper, config.RoleUIDesigner}) {
		t.Fatalf("node roles = %v, want [developer ui-designer]", node.Roles)
	}
	if node.Speed != "fast" {
		t.Fatalf("node speed = %q, want fast", node.Speed)
	}
	if !node.Handoff {
		t.Fatalf("node handoff = false, want true")
	}
	if len(node.Children) != 1 {
		t.Fatalf("len(node children) = %d, want 1", len(node.Children))
	}
	if !equalStrings(node.Children[0].Roles, []string{config.RoleScout}) {
		t.Fatalf("child roles = %v, want [scout]", node.Children[0].Roles)
	}
}

func TestNodesToDelegationConfigSerializesSingleAndMultiRoles(t *testing.T) {
	cfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(cfg)

	nodes := []*loopDelegationNode{
		{
			Profile: "worker",
			Roles:   []string{config.RoleDeveloper, config.RoleUIDesigner},
		},
		{
			Profile: "defaulted",
			Roles:   nil,
		},
		{
			Profile: "lead",
			Roles:   []string{config.RoleLeadDeveloper},
		},
	}

	got := nodesToDelegationConfig(nodes, cfg)
	if got == nil {
		t.Fatalf("nodesToDelegationConfig() = nil, want non-nil")
	}
	if len(got.Profiles) != 3 {
		t.Fatalf("len(profiles) = %d, want 3", len(got.Profiles))
	}
	if got.Profiles[0].Role != "" {
		t.Fatalf("profiles[0].Role = %q, want empty for multi-role entry", got.Profiles[0].Role)
	}
	if !equalStrings(got.Profiles[0].Roles, []string{config.RoleDeveloper, config.RoleUIDesigner}) {
		t.Fatalf("profiles[0].Roles = %v, want [developer ui-designer]", got.Profiles[0].Roles)
	}

	defaultRole := config.DefaultRole(cfg)
	if got.Profiles[1].Role != defaultRole {
		t.Fatalf("profiles[1].Role = %q, want %q", got.Profiles[1].Role, defaultRole)
	}
	if len(got.Profiles[1].Roles) != 0 {
		t.Fatalf("profiles[1].Roles = %v, want empty", got.Profiles[1].Roles)
	}

	if got.Profiles[2].Role != config.RoleLeadDeveloper {
		t.Fatalf("profiles[2].Role = %q, want %q", got.Profiles[2].Role, config.RoleLeadDeveloper)
	}
	if len(got.Profiles[2].Roles) != 0 {
		t.Fatalf("profiles[2].Roles = %v, want empty", got.Profiles[2].Roles)
	}
}

func TestUpdateLoopStepSpawnROpensRolePicker(t *testing.T) {
	cfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(cfg)

	m := AppModel{
		state:     stateLoopStepSpawn,
		globalCfg: cfg,
		loopStepDelegRoots: []*loopDelegationNode{
			{
				Profile: "worker",
				Roles:   []string{config.RoleUIDesigner, config.RoleDeveloper},
			},
		},
	}

	updated, _ := m.updateLoopStepSpawn(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := updated.(AppModel)
	if got.state != stateLoopStepSpawnRoles {
		t.Fatalf("state = %v, want %v", got.state, stateLoopStepSpawnRoles)
	}

	roles := config.AllRoles(cfg)
	wantSel := 0
	for i, role := range roles {
		if role == config.RoleUIDesigner {
			wantSel = i
			break
		}
	}
	if got.loopStepSpawnRoleSel != wantSel {
		t.Fatalf("loopStepSpawnRoleSel = %d, want %d", got.loopStepSpawnRoleSel, wantSel)
	}
}

func TestUpdateLoopStepSpawnRolesToggleKeepsAtLeastOneRole(t *testing.T) {
	cfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(cfg)

	m := AppModel{
		state:     stateLoopStepSpawnRoles,
		globalCfg: cfg,
		loopStepDelegRoots: []*loopDelegationNode{
			{
				Profile: "worker",
				Roles:   []string{config.RoleDeveloper},
			},
		},
	}

	devIdx := roleIndex(config.AllRoles(cfg), config.RoleDeveloper)
	if devIdx < 0 {
		t.Fatalf("developer role missing in role catalog")
	}
	m.loopStepSpawnRoleSel = devIdx

	updated, _ := m.updateLoopStepSpawnRoles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got := updated.(AppModel)
	if !equalStrings(got.loopStepDelegRoots[0].Roles, []string{config.RoleDeveloper}) {
		t.Fatalf("roles after removing last role = %v, want [developer]", got.loopStepDelegRoots[0].Roles)
	}

	uiIdx := roleIndex(config.AllRoles(cfg), config.RoleUIDesigner)
	if uiIdx < 0 {
		t.Fatalf("ui-designer role missing in role catalog")
	}
	got.loopStepSpawnRoleSel = uiIdx
	updated, _ = got.updateLoopStepSpawnRoles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got = updated.(AppModel)
	if !equalStrings(got.loopStepDelegRoots[0].Roles, []string{config.RoleDeveloper, config.RoleUIDesigner}) {
		t.Fatalf("roles after add = %v, want [developer ui-designer]", got.loopStepDelegRoots[0].Roles)
	}

	got.loopStepSpawnRoleSel = devIdx
	updated, _ = got.updateLoopStepSpawnRoles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	got = updated.(AppModel)
	if !equalStrings(got.loopStepDelegRoots[0].Roles, []string{config.RoleUIDesigner}) {
		t.Fatalf("roles after removing developer = %v, want [ui-designer]", got.loopStepDelegRoots[0].Roles)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func roleIndex(roles []string, target string) int {
	for i, role := range roles {
		if role == target {
			return i
		}
	}
	return -1
}
