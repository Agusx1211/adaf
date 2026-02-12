package prompt

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestDelegationSection_IncludesQuickstartWhenDelegationEnabled(t *testing.T) {
	got := delegationSection(&config.DelegationConfig{}, nil)

	hasQuickstart := strings.Contains(got, "Quick scheduling flow:")
	if !hasQuickstart {
		t.Fatalf("expected quickstart guidance in delegation section\nprompt:\n%s", got)
	}

	hasWaitFlow := strings.Contains(got, "adaf wait-for-spawns")
	if !hasWaitFlow {
		t.Fatalf("expected quickstart guidance to include wait-for-spawns flow\nprompt:\n%s", got)
	}
}

func TestDelegationSection_NoDelegation(t *testing.T) {
	got := delegationSection(nil, nil)
	if got != "You cannot spawn sub-agents.\n\n" {
		t.Fatalf("delegationSection(nil) = %q, want %q", got, "You cannot spawn sub-agents.\n\n")
	}
}
