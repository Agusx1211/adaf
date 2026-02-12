package protocol

import (
	"strings"
	"testing"
)

func TestAgentInstructions_DelegationFlowGuidance(t *testing.T) {
	got := AgentInstructions("demo")

	if !strings.Contains(got, "spawn independent tasks without") {
		t.Fatalf("expected preferred non-blocking spawn guidance in instructions\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "adaf wait-for-spawns") {
		t.Fatalf("expected wait-for-spawns guidance in instructions\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Avoid repeatedly polling `adaf spawn-status`") {
		t.Fatalf("expected anti-polling guidance in instructions\nprompt:\n%s", got)
	}
}
