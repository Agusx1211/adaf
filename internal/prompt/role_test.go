package prompt

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestDelegationSection_IncludesSkillPointerWhenDelegationEnabled(t *testing.T) {
	got := delegationSection(&config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker"},
		},
	}, nil, nil)

	if !strings.Contains(got, "# Delegation") {
		t.Fatalf("expected delegation header\nprompt:\n%s", got)
	}

	if !strings.Contains(got, "adaf skill delegation") {
		t.Fatalf("expected pointer to adaf skill delegation\nprompt:\n%s", got)
	}

	if !strings.Contains(got, "Maximum concurrent sub-agents") {
		t.Fatalf("expected max parallel info\nprompt:\n%s", got)
	}
}

func TestDelegationSection_NoDelegation(t *testing.T) {
	got := delegationSection(nil, nil, nil)
	if got != "" {
		t.Fatalf("delegationSection(nil) = %q, want empty", got)
	}

	got = delegationSection(&config.DelegationConfig{}, nil, nil)
	if got != "" {
		t.Fatalf("delegationSection(empty) = %q, want empty", got)
	}
}

func TestDelegationSection_IncludesRoleDetails(t *testing.T) {
	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{
				Name:     "worker",
				Position: config.PositionWorker,
				Role:     config.RoleDeveloper,
			},
		},
	}
	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "worker", Agent: "codex"},
		},
	}

	got := delegationSection(deleg, globalCfg, nil)
	if !strings.Contains(got, "role=developer") {
		t.Fatalf("expected role annotation in delegation section\nprompt:\n%s", got)
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

func TestRolePrompt_ComposesRulesFromCatalog(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Roles: []config.RoleDefinition{
			{
				Name:         "reviewer",
				Title:        "REVIEWER",
				Description:  "Read and assess changes.",
				CanWriteCode: false,
				RuleIDs:      []string{"r1", "r2"},
			},
		},
		PromptRules: []config.PromptRule{
			{ID: "r1", Body: "Rule one body."},
			{ID: "r2", Body: "Rule two body."},
		},
		DefaultRole: "reviewer",
	}
	got := RolePrompt(&config.Profile{Name: "p1"}, "reviewer", globalCfg)

	if !strings.Contains(got, "# Your Role: REVIEWER") {
		t.Fatalf("expected role title in prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Rule one body.") {
		t.Fatalf("expected first rule body in prompt\nprompt:\n%s", got)
	}
	if !strings.Contains(got, "Rule two body.") {
		t.Fatalf("expected second rule body in prompt\nprompt:\n%s", got)
	}
}

func TestRolePrompt_DoesNotRenderDownstreamCommunicationRule(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Roles: []config.RoleDefinition{
			{
				Name:         "developer",
				Title:        "DEVELOPER",
				Description:  "Executes implementation.",
				CanWriteCode: true,
				RuleIDs:      []string{"dev_rule", config.RuleCommunicationDownstream},
			},
		},
		PromptRules: []config.PromptRule{
			{ID: "dev_rule", Body: "Developer identity."},
			{ID: config.RuleCommunicationDownstream, Body: "## Communication Style: Downstream Only\n\n- `adaf spawn-message --spawn-id 1 \"fix\"`"},
		},
		DefaultRole: "developer",
	}

	got := RolePrompt(&config.Profile{Name: "p1"}, "developer", globalCfg)
	if strings.Contains(got, "Communication Style: Downstream Only") {
		t.Fatalf("downstream communication should be emitted only in delegation context\nprompt:\n%s", got)
	}
	if strings.Contains(got, "adaf spawn-message") {
		t.Fatalf("spawn communication commands should come from delegation context, not role prompt\nprompt:\n%s", got)
	}
}

func TestRolePrompt_RendersRoleIdentityFromRoleDefinition(t *testing.T) {
	globalCfg := &config.GlobalConfig{
		Roles: []config.RoleDefinition{
			{
				Name:         "qa",
				Title:        "QA",
				Identity:     "You are a QA role focused on high-signal verification.",
				Description:  "Verification specialist.",
				CanWriteCode: true,
			},
		},
		PromptRules: []config.PromptRule{
			{ID: "shared_checks", Body: "Always include repro steps."},
		},
		DefaultRole: "qa",
	}

	got := RolePrompt(&config.Profile{Name: "p1"}, "qa", globalCfg)
	if !strings.Contains(got, "You are a QA role focused on high-signal verification.") {
		t.Fatalf("role identity should render from role definition\nprompt:\n%s", got)
	}
}
