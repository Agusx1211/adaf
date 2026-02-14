package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/config"
)

func TestDigitsOnly(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{name: "empty", input: "", maxLen: 2, want: ""},
		{name: "filters non digits", input: "a1-b2", maxLen: 4, want: "12"},
		{name: "limits length", input: "12345", maxLen: 3, want: "123"},
		{name: "zero max len", input: "123", maxLen: 0, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := digitsOnly(tt.input, tt.maxLen); got != tt.want {
				t.Fatalf("digitsOnly(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestUpdateProfileIntelFiltersInput(t *testing.T) {
	m := AppModel{globalCfg: &config.GlobalConfig{}, state: stateProfileIntel}

	model, _ := m.updateProfileIntel(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = model.(AppModel)
	if m.profileWiz.IntelInput != "" {
		t.Fatalf("profileIntelInput after non-digit = %q, want empty", m.profileWiz.IntelInput)
	}

	for _, r := range []rune{'1', '2', '3'} {
		model, _ = m.updateProfileIntel(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = model.(AppModel)
	}
	if m.profileWiz.IntelInput != "12" {
		t.Fatalf("profileIntelInput = %q, want %q", m.profileWiz.IntelInput, "12")
	}
}

func TestUpdateSettingsRuleBodyTextarea(t *testing.T) {
	m := AppModel{
		globalCfg: &config.GlobalConfig{
			PromptRules: []config.PromptRule{{ID: "rule_a", Body: ""}},
		},
		state: stateSettingsRuleBody,
		settings: SettingsState{
			EditRuleIdx: 0,
		},
	}

	for _, msg := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}},
	} {
		model, _ := m.updateSettingsRuleBody(msg)
		m = model.(AppModel)
	}

	if m.settings.RuleBodyInput != "ab\nc" {
		t.Fatalf("settingsRuleBodyInput = %q, want %q", m.settings.RuleBodyInput, "ab\\nc")
	}

	model, _ := m.updateSettingsRuleBody(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(AppModel)
	if m.state != stateSettingsRulesList {
		t.Fatalf("state after esc = %v, want %v", m.state, stateSettingsRulesList)
	}
}

func TestUpdateSettingsRuleBodyAllowsUppercaseS(t *testing.T) {
	m := AppModel{
		globalCfg: &config.GlobalConfig{
			PromptRules: []config.PromptRule{{ID: "rule_a", Body: ""}},
		},
		state: stateSettingsRuleBody,
		settings: SettingsState{
			EditRuleIdx: 0,
		},
	}

	model, _ := m.updateSettingsRuleBody(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	m = model.(AppModel)

	if m.settings.RuleBodyInput != "S" {
		t.Fatalf("settingsRuleBodyInput = %q, want %q", m.settings.RuleBodyInput, "S")
	}
	if m.state != stateSettingsRuleBody {
		t.Fatalf("state = %v, want %v", m.state, stateSettingsRuleBody)
	}
}

func TestRuleBodyEditorContextRoleIdentity(t *testing.T) {
	m := AppModel{
		globalCfg: &config.GlobalConfig{
			Roles: []config.RoleDefinition{{Name: "reviewer"}},
		},
		settings: SettingsState{
			EditRuleIdx: -1,
			EditRoleIdx: 0,
		},
	}

	key, editingRoleIdentity, ok := m.ruleBodyEditorContext()
	if !ok {
		t.Fatalf("ruleBodyEditorContext returned ok=false")
	}
	if !editingRoleIdentity {
		t.Fatalf("editingRoleIdentity = false, want true")
	}
	if key == "" {
		t.Fatalf("editor key is empty")
	}
}

func TestUpdateProfileNameForwardsNonKeyMsgs(t *testing.T) {
	m := AppModel{
		globalCfg: &config.GlobalConfig{},
		state:     stateProfileName,
	}

	if cmd := m.initInputEditorForState(); cmd == nil {
		t.Fatalf("initInputEditorForState returned nil cmd for profile-name")
	}

	model, cmd := m.updateProfileName(cursor.Blink())
	m = model.(AppModel)
	if cmd == nil {
		t.Fatalf("updateProfileName non-key msg returned nil cmd; expected forwarded cursor cmd")
	}
	if m.state != stateProfileName {
		t.Fatalf("state = %v, want %v", m.state, stateProfileName)
	}
}

func TestSyncScrollStateResetsInputEditorsOnStateChange(t *testing.T) {
	m := AppModel{
		state:        stateProfileName,
		textInputKey: "profile-name",
		textAreaKey:  "settings-rule-body-1",
	}

	m.syncScrollState(stateSelector)

	if m.textInputKey != "" {
		t.Fatalf("textInputKey = %q, want empty", m.textInputKey)
	}
	if m.textAreaKey != "" {
		t.Fatalf("textAreaKey = %q, want empty", m.textAreaKey)
	}
}
