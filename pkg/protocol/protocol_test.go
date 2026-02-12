package protocol

import (
	"strings"
	"testing"
)

func TestAgentInstructions_DelegationFlowGuidance(t *testing.T) {
	got := AgentInstructions("demo")

	if !strings.Contains(got, "Spawn all independent tasks at once") {
		t.Fatalf("expected preferred non-blocking spawn guidance in instructions\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "adaf wait-for-spawns") {
		t.Fatalf("expected wait-for-spawns guidance in instructions\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "keeps your session alive and wastes tokens") {
		t.Fatalf("expected anti-wait guidance in instructions\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "--read-only") {
		t.Fatalf("expected read-only scout guidance in instructions\nprompt:\n%s", got)
	}
}
