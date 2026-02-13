package prompt

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestDelegationSection_IncludesQuickstartWhenDelegationEnabled(t *testing.T) {
	got := delegationSection(&config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker"},
		},
	}, nil)

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

	got = delegationSection(&config.DelegationConfig{}, nil)
	if got != "You cannot spawn sub-agents.\n\n" {
		t.Fatalf("delegationSection(empty) = %q, want %q", got, "You cannot spawn sub-agents.\n\n")
	}
}

func TestDelegationSection_IncludesRoleDetails(t *testing.T) {
	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{
				Name: "worker",
				Role: config.RoleSenior,
				Delegation: &config.DelegationConfig{
					Profiles: []config.DelegationProfile{{Name: "scout", Role: config.RoleJunior}},
				},
			},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "worker", Agent: "codex"},
		},
	}

	got := delegationSection(deleg, globalCfg)
	if !strings.Contains(got, "role=senior") {
		t.Fatalf("expected role annotation in delegation section\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "[child-spawn:1]") {
		t.Fatalf("expected child-spawn annotation in delegation section\nprompt:\n%s", got)
	}
}

func TestReadOnlyPrompt_RequiresFinalMessageReport(t *testing.T) {
	got := ReadOnlyPrompt()

	if !strings.Contains(got, "Do NOT write reports into repository files") {
		t.Fatalf("expected read-only prompt to forbid writing reports to repo files\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "final assistant message") {
		t.Fatalf("expected read-only prompt to require final assistant message reporting\nprompt:\n%s", got)
	}
}
