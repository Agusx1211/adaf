package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/store"
)

func TestUpdateByStateDispatch(t *testing.T) {
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")
	t.Setenv("ADAF_AGENT", "")
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	m := NewApp(s)

	states := []appState{
		stateSelector,
		statePlanMenu,
		statePlanCreateID,
		statePlanCreateTitle,
		stateProfileName,
		stateProfileAgent,
		stateProfileModel,
		stateProfileReasoning,
		stateProfileIntel,
		stateProfileDesc,
		stateProfileMaxInst,
		stateProfileSpeed,
		stateProfileMenu,
		stateLoopName,
		stateLoopStepList,
		stateLoopStepProfile,
		stateLoopStepRole,
		stateLoopStepTurns,
		stateLoopStepInstr,
		stateLoopStepTools,
		stateLoopStepSpawn,
		stateLoopStepSpawnCfg,
		stateLoopStepSpawnRoles,
		stateLoopMenu,
		stateSettings,
		stateSettingsPushoverUserKey,
		stateSettingsPushoverAppToken,
		stateSettingsRolesRulesMenu,
		stateSettingsRolesList,
		stateSettingsRoleName,
		stateSettingsRoleEdit,
		stateSettingsRulesList,
		stateSettingsRuleID,
		stateSettingsRuleBody,
		stateSessionPicker,
		stateConfirmDelete,
	}

	for _, state := range states {
		t.Run(fmt.Sprintf("state_%d", state), func(t *testing.T) {
			m.state = state
			// Send a dummy key message to trigger updateByState.
			// We don't necessarily care about the outcome here, just that it doesn't panic
			// and returns a model.
			updated, _ := m.updateByState(tea.KeyMsg{Type: tea.KeyEsc})
			if updated == nil {
				t.Errorf("state %v: updateByState returned nil", state)
			}
		})
	}
}

func TestStateTransitions(t *testing.T) {
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")
	t.Setenv("ADAF_AGENT", "")
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	tests := []struct {
		name      string
		fromState appState
		key       string
		setup     func(*AppModel)
		wantState appState
	}{
		// Selector transitions
		{"selector n opens profile name", stateSelector, "n", nil, stateProfileName},
		{"selector l opens loop name", stateSelector, "l", nil, stateLoopName},
		{"selector S opens settings", stateSelector, "S", nil, stateSettings},
		{"selector p opens plan menu", stateSelector, "p", nil, statePlanMenu},

		// Profile wizard forward navigation
		{"profile name enter goes to agent", stateProfileName, "enter", func(m *AppModel) { m.profileWiz.NameInput = "test" }, stateProfileAgent},
		{"profile agent enter goes to model", stateProfileAgent, "enter", func(m *AppModel) { m.profileWiz.Agents = []string{"claude"}; m.profileWiz.AgentSel = 0 }, stateProfileModel},
		{"profile model enter goes to reasoning or intel", stateProfileModel, "enter", func(m *AppModel) {
			m.profileWiz.Agents = []string{"claude"}
			m.profileWiz.AgentSel = 0
			m.profileWiz.Models = []string{"sonnet"}
			m.profileWiz.ModelSel = 0
		}, stateProfileReasoning},
		{"profile reasoning enter goes to intel", stateProfileReasoning, "enter", func(m *AppModel) {
			m.profileWiz.Agents = []string{"claude"}
			m.profileWiz.AgentSel = 0
			m.profileWiz.ReasoningLevels = []agentmeta.ReasoningLevel{{Name: "high"}}
			m.profileWiz.ReasoningLevelSel = 0
		}, stateProfileIntel},
		{"profile intel enter goes to desc", stateProfileIntel, "enter", nil, stateProfileDesc},
		{"profile desc enter goes to max inst", stateProfileDesc, "enter", nil, stateProfileMaxInst},
		{"profile max inst enter goes to speed", stateProfileMaxInst, "enter", nil, stateProfileSpeed},
		{"profile speed enter returns to selector", stateProfileSpeed, "enter", func(m *AppModel) {
			m.profileWiz.Agents = []string{"claude"}
			m.profileWiz.AgentSel = 0
			m.profileWiz.NameInput = "test"
		}, stateSelector},

		// Profile wizard backward navigation
		{"profile name esc returns to selector", stateProfileName, "esc", nil, stateSelector},
		{"profile agent esc returns to name", stateProfileAgent, "esc", nil, stateProfileName},
		{"profile model esc returns to agent", stateProfileModel, "esc", nil, stateProfileAgent},
		{"profile reasoning esc returns to model", stateProfileReasoning, "esc", nil, stateProfileModel},
		{"profile intel esc returns to reasoning", stateProfileIntel, "esc", func(m *AppModel) {
			m.profileWiz.Agents = []string{"claude"}
			m.profileWiz.AgentSel = 0
			m.profileWiz.ReasoningLevels = []agentmeta.ReasoningLevel{{Name: "high"}}
		}, stateProfileReasoning},
		{"profile desc esc returns to intel", stateProfileDesc, "esc", nil, stateProfileIntel},
		{"profile max inst esc returns to desc", stateProfileMaxInst, "esc", nil, stateProfileDesc},
		{"profile speed esc returns to max inst", stateProfileSpeed, "esc", nil, stateProfileMaxInst},

		// Loop wizard navigation
		{"loop name enter goes to step list", stateLoopName, "enter", func(m *AppModel) { m.loopWiz.NameInput = "test" }, stateLoopStepList},
		{"loop name esc returns to selector", stateLoopName, "esc", nil, stateSelector},
		{"loop step list a goes to step profile", stateLoopStepList, "a", nil, stateLoopStepProfile},
		{"loop step list esc returns to name", stateLoopStepList, "esc", nil, stateLoopName},
		{"loop step profile enter goes to step role", stateLoopStepProfile, "enter", func(m *AppModel) { m.loopWiz.StepProfileOpts = []string{"p1"}; m.loopWiz.StepProfileSel = 0 }, stateLoopStepRole},
		{"loop step role enter goes to step turns", stateLoopStepRole, "enter", func(m *AppModel) { m.globalCfg.Roles = []config.RoleDefinition{{Name: "r1"}} }, stateLoopStepTurns},
		{"loop step turns enter goes to step instr", stateLoopStepTurns, "enter", nil, stateLoopStepInstr},
		{"loop step instr enter goes to step tools", stateLoopStepInstr, "enter", nil, stateLoopStepTools},
		{"loop step tools enter goes to step spawn", stateLoopStepTools, "enter", nil, stateLoopStepSpawn},
		{"loop step spawn a goes to step spawn cfg", stateLoopStepSpawn, "a", func(m *AppModel) { m.loopWiz.StepSpawnOpts = []string{"p1"} }, stateLoopStepSpawnCfg},
		{"loop step spawn cfg enter returns to step spawn", stateLoopStepSpawnCfg, "enter", func(m *AppModel) { m.loopWiz.StepSpawnOpts = []string{"p1"}; m.loopWiz.StepSpawnCfgSel = 0 }, stateLoopStepSpawn},

		// Settings transitions
		{"settings esc returns to selector", stateSettings, "esc", nil, stateSelector},
		{"settings enter opens pushover user", stateSettings, "enter", func(m *AppModel) { m.settings.Sel = 0 }, stateSettingsPushoverUserKey},
		{"settings pushover user esc returns to settings", stateSettingsPushoverUserKey, "esc", nil, stateSettings},
		{"settings roles rules menu enters roles list", stateSettingsRolesRulesMenu, "enter", func(m *AppModel) { m.settings.RolesRulesSel = 0 }, stateSettingsRolesList},
		{"settings roles rules menu enters rules list", stateSettingsRolesRulesMenu, "enter", func(m *AppModel) { m.settings.RolesRulesSel = 1 }, stateSettingsRulesList},
		{"settings roles list a goes to role name", stateSettingsRolesList, "a", nil, stateSettingsRoleName},
		{"settings roles list enter goes to role edit", stateSettingsRolesList, "enter", func(m *AppModel) { m.globalCfg.Roles = []config.RoleDefinition{{Name: "r1"}}; m.settings.RolesSel = 0 }, stateSettingsRoleEdit},
		{"settings role edit esc returns to roles list", stateSettingsRoleEdit, "esc", func(m *AppModel) {
			m.globalCfg.Roles = []config.RoleDefinition{{Name: "r1"}}
			m.settings.EditRoleIdx = 0
		}, stateSettingsRolesList},
		{"settings rules list a goes to rule id", stateSettingsRulesList, "a", nil, stateSettingsRuleID},
		{"settings rule id enter goes to rule body", stateSettingsRuleID, "enter", func(m *AppModel) { m.settings.RuleIDInput = "new-rule"; m.settings.EditRuleIdx = -1 }, stateSettingsRuleBody},
		{"settings rule body esc returns to rules list", stateSettingsRuleBody, "esc", func(m *AppModel) {
			m.settings.EditRuleIdx = 0
			m.globalCfg.PromptRules = []config.PromptRule{{ID: "r1"}}
		}, stateSettingsRulesList},
		{"settings rule body ctrl+s returns to rules list", stateSettingsRuleBody, "ctrl+s", func(m *AppModel) {
			m.settings.EditRuleIdx = 0
			m.globalCfg.PromptRules = []config.PromptRule{{ID: "r1"}}
		}, stateSettingsRulesList},
		{"plan menu esc returns to selector", statePlanMenu, "esc", nil, stateSelector},
		{"plan menu c opens plan create id", statePlanMenu, "c", nil, statePlanCreateID},
		{"plan create id enter goes to title", statePlanCreateID, "enter", func(m *AppModel) { m.planWiz.CreateIDInput = "test-plan" }, statePlanCreateTitle},
		{"plan create title esc returns to id", statePlanCreateTitle, "esc", nil, statePlanCreateID},

		// Confirm delete transitions
		{"confirm delete n returns to selector", stateConfirmDelete, "n", nil, stateSelector},
		{"confirm delete esc returns to selector", stateConfirmDelete, "esc", nil, stateSelector},

		// Session picker transitions
		{"session picker esc returns to selector", stateSessionPicker, "esc", nil, stateSelector},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewApp(s)
			m.state = tt.fromState
			if tt.setup != nil {
				tt.setup(&m)
			}

			var msg tea.Msg
			if tt.key == "esc" {
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			} else if tt.key == "enter" {
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			} else if tt.key == "tab" {
				msg = tea.KeyMsg{Type: tea.KeyTab}
			} else if tt.key == "shift+tab" {
				msg = tea.KeyMsg{Type: tea.KeyShiftTab}
			} else if tt.key == "ctrl+s" {
				msg = tea.KeyMsg{Type: tea.KeyCtrlS}
			} else {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			}

			updated, _ := m.Update(msg)

			got, ok := updated.(AppModel)
			if !ok {
				t.Fatalf("Update did not return AppModel")
			}

			if got.state != tt.wantState {
				t.Errorf("got state %v, want %v", got.state, tt.wantState)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")
	t.Setenv("ADAF_AGENT", "")
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	m := NewApp(s)

	t.Run("Unknown state does not panic", func(t *testing.T) {
		m.state = appState(999)
		updated, _ := m.updateByState(tea.KeyMsg{Type: tea.KeyEsc})
		if updated == nil {
			t.Fatal("updated model is nil")
		}
	})

	t.Run("Window resize handled in all states", func(t *testing.T) {
		states := []appState{stateSelector, stateRunning, stateSettings}
		for _, st := range states {
			m.state = st
			if st == stateRunning {
				m.runModel = runtui.Model{} // minimal run model
			}
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
			got := updated.(AppModel)
			if got.width != 100 || got.height != 50 {
				t.Errorf("state %v: dimensions not updated", st)
			}
		}
	})
}
