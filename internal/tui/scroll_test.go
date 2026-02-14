package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/config"
)

func TestHandleGlobalScrollKeyRequiresRightFocusOnSettingsSplit(t *testing.T) {
	m := AppModel{
		state:         stateSettingsRulesList,
		width:         120,
		height:        30,
		globalCfg:     &config.GlobalConfig{},
		viewScroll:    map[appState]int{},
		viewPaneFocus: map[appState]bool{},
	}
	config.EnsureDefaultRoleCatalog(m.globalCfg)

	if handled := m.handleGlobalScrollKey(tea.KeyMsg{Type: tea.KeyPgDown}); handled {
		t.Fatal("expected pgdown to be ignored when right pane is not focused")
	}
	if got := m.stateScrollOffset(); got != 0 {
		t.Fatalf("scroll offset = %d, want 0", got)
	}

	m.setRightPaneFocused(true)
	if handled := m.handleGlobalScrollKey(tea.KeyMsg{Type: tea.KeyPgDown}); !handled {
		t.Fatal("expected pgdown to scroll when right pane is focused")
	}
	if got := m.stateScrollOffset(); got <= 0 {
		t.Fatalf("scroll offset = %d, want > 0", got)
	}
}

func TestUpdateSettingsRulesListJScrollsRightPaneWhenFocused(t *testing.T) {
	m := AppModel{
		state:  stateSettingsRulesList,
		width:  120,
		height: 30,
		globalCfg: &config.GlobalConfig{
			PromptRules: []config.PromptRule{
				{ID: "alpha", Body: "A"},
				{ID: "beta", Body: "B"},
			},
		},
		viewScroll:    map[appState]int{},
		viewPaneFocus: map[appState]bool{},
	}
	config.EnsureDefaultRoleCatalog(m.globalCfg)
	m.setRightPaneFocused(true)

	updated, _ := m.updateSettingsRulesList(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := updated.(AppModel)
	if got.settings.RulesSel != 0 {
		t.Fatalf("settingsRulesSel = %d, want 0", got.settings.RulesSel)
	}
	if got.stateScrollOffset() <= 0 {
		t.Fatalf("scroll offset = %d, want > 0", got.stateScrollOffset())
	}
}

func TestUpdateLoopStepRoleJScrollsPromptWhenRightFocused(t *testing.T) {
	cfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(cfg)

	m := AppModel{
		state:         stateLoopStepRole,
		width:         120,
		height:        30,
		globalCfg:     cfg,
		viewScroll:    map[appState]int{},
		viewPaneFocus: map[appState]bool{},
		loopWiz: LoopWizardState{
			StepRoleSel: 0,
		},
	}
	m.setRightPaneFocused(true)

	updated, _ := m.updateLoopStepRole(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := updated.(AppModel)
	if got.loopWiz.StepRoleSel != 0 {
		t.Fatalf("loopStepRoleSel = %d, want 0", got.loopWiz.StepRoleSel)
	}
	if got.stateScrollOffset() <= 0 {
		t.Fatalf("scroll offset = %d, want > 0", got.stateScrollOffset())
	}
}

func TestUpdateLoopStepSpawnRolesJScrollsPromptWhenRightFocused(t *testing.T) {
	cfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(cfg)

	m := AppModel{
		state:         stateLoopStepSpawnRoles,
		width:         120,
		height:        30,
		globalCfg:     cfg,
		viewScroll:    map[appState]int{},
		viewPaneFocus: map[appState]bool{},
		loopWiz: LoopWizardState{
			StepDelegRoots: []*loopDelegationNode{
				{
					Profile: "worker",
					Roles:   []string{config.RoleDeveloper},
				},
			},
			StepSpawnRoleSel: 0,
		},
	}
	m.setRightPaneFocused(true)

	updated, _ := m.updateLoopStepSpawnRoles(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := updated.(AppModel)
	if got.loopWiz.StepSpawnRoleSel != 0 {
		t.Fatalf("loopStepSpawnRoleSel = %d, want 0", got.loopWiz.StepSpawnRoleSel)
	}
	if got.stateScrollOffset() <= 0 {
		t.Fatalf("scroll offset = %d, want > 0", got.stateScrollOffset())
	}
}

func TestUpdateSelectorJScrollsDetailsWhenRightFocused(t *testing.T) {
	m := AppModel{
		state:  stateSelector,
		width:  120,
		height: 30,
		profiles: []profileEntry{
			{Name: "alpha"},
			{Name: "beta"},
		},
		viewScroll:    map[appState]int{},
		viewPaneFocus: map[appState]bool{},
	}
	m.setRightPaneFocused(true)

	updated, _ := m.updateSelector(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := updated.(AppModel)
	if got.selector.Selected != 0 {
		t.Fatalf("selected = %d, want 0", got.selector.Selected)
	}
	if got.stateScrollOffset() <= 0 {
		t.Fatalf("scroll offset = %d, want > 0", got.stateScrollOffset())
	}
}
