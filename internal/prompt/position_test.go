package prompt

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestPositionPrompt_ManagerRequiresTurnLogLanguage(t *testing.T) {
	got := PositionPrompt(config.PositionManager, "", true)
	if !strings.Contains(got, "MUST publish a manager handoff") {
		t.Fatalf("manager prompt should require turn log\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "`adaf spawn --profile ... --task ...`") {
		t.Fatalf("manager prompt should include delegation commands when delegation exists\nprompt:\n%s", got)
	}
}

func TestPositionPrompt_LeadWithoutTeamOmitsDelegationCommands(t *testing.T) {
	got := PositionPrompt(config.PositionLead, "", false)
	if strings.Contains(got, "Delegation: `adaf spawn ...`") {
		t.Fatalf("lead without team should not get delegation commands\nprompt:\n%s", got)
	}
}

func TestPositionPrompt_SupervisorIncludesLoopMessageGuidance(t *testing.T) {
	got := PositionPrompt(config.PositionSupervisor, "", false)
	if !strings.Contains(got, "`adaf loop message \"guidance\"`") {
		t.Fatalf("supervisor prompt should include loop message guidance\nprompt:\n%s", got)
	}
}
