package prompt

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestDelegationSection_IncludesQuickstartWhenDelegationEnabled(t *testing.T) {
	got := delegationSection(&config.DelegationConfig{}, nil)

	hasSpawnFlow := strings.Contains(got, "ALWAYS use this pattern")
	if !hasSpawnFlow {
		t.Fatalf("expected spawn flow guidance in delegation section\nprompt:\n%s", got)
	}

	hasWaitFlow := strings.Contains(got, "adaf wait-for-spawns")
	if !hasWaitFlow {
		t.Fatalf("expected quickstart guidance to include wait-for-spawns flow\nprompt:\n%s", got)
	}

	hasAntiWait := strings.Contains(got, "burns tokens while you idle")
	if !hasAntiWait {
		t.Fatalf("expected guidance against --wait in delegation section\nprompt:\n%s", got)
	}

	hasScouts := strings.Contains(got, "Scouts (read-only sub-agents)")
	if !hasScouts {
		t.Fatalf("expected scout guidance in delegation section\nprompt:\n%s", got)
	}
}

func TestDelegationSection_NoDelegation(t *testing.T) {
	got := delegationSection(nil, nil)
	if got != "You cannot spawn sub-agents.\n\n" {
		t.Fatalf("delegationSection(nil) = %q, want %q", got, "You cannot spawn sub-agents.\n\n")
	}
}
