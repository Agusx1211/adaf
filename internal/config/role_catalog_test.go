package config

import (
	"strings"
	"testing"
)

func TestEnsureDefaultRoleCatalog(t *testing.T) {
	cfg := &GlobalConfig{}
	changed := EnsureDefaultRoleCatalog(cfg)
	if !changed {
		t.Fatalf("EnsureDefaultRoleCatalog() changed = false, want true")
	}
	if len(cfg.Roles) == 0 {
		t.Fatalf("roles should be seeded")
	}
	if len(cfg.PromptRules) == 0 {
		t.Fatalf("prompt rules should be seeded")
	}
	if cfg.DefaultRole == "" {
		t.Fatalf("default role should be set")
	}
}

func TestCustomRoleCatalogAndWritePolicy(t *testing.T) {
	cfg := &GlobalConfig{
		Roles: []RoleDefinition{
			{
				Name:         "reviewer",
				CanWriteCode: false,
				RuleIDs:      []string{"review_rule"},
			},
			{
				Name:         "implementer",
				CanWriteCode: true,
				RuleIDs:      []string{"impl_rule"},
			},
		},
		PromptRules: []PromptRule{
			{ID: "review_rule", Body: "Review-only behavior."},
			{ID: "impl_rule", Body: "Implementation behavior."},
		},
		DefaultRole: "reviewer",
	}
	EnsureDefaultRoleCatalog(cfg)

	if got := EffectiveStepRole("", cfg); got != "reviewer" {
		t.Fatalf("EffectiveStepRole(empty) = %q, want %q", got, "reviewer")
	}
	if got := EffectiveStepRole("implementer", cfg); got != "implementer" {
		t.Fatalf("EffectiveStepRole(implementer) = %q, want %q", got, "implementer")
	}
	if CanWriteCode("reviewer", cfg) {
		t.Fatalf("CanWriteCode(reviewer) = true, want false")
	}
	if !CanWriteCode("implementer", cfg) {
		t.Fatalf("CanWriteCode(implementer) = false, want true")
	}
}

func TestRemovePromptRuleUnlinksFromRoles(t *testing.T) {
	cfg := &GlobalConfig{
		Roles: []RoleDefinition{
			{Name: "reviewer", RuleIDs: []string{"a", "b"}},
		},
		PromptRules: []PromptRule{
			{ID: "a", Body: "A"},
			{ID: "b", Body: "B"},
		},
		DefaultRole: "reviewer",
	}
	EnsureDefaultRoleCatalog(cfg)

	cfg.RemovePromptRule("a")

	if cfg.FindPromptRule("a") != nil {
		t.Fatalf("prompt rule a should be removed")
	}
	role := cfg.FindRoleDefinition("reviewer")
	if role == nil {
		t.Fatalf("role reviewer missing after removal")
	}
	if len(role.RuleIDs) != 1 || role.RuleIDs[0] != "b" {
		t.Fatalf("role rule IDs = %v, want [b]", role.RuleIDs)
	}
}

func TestDefaultRoleDefinitions_DoNotFixUpstreamCommunicationByRole(t *testing.T) {
	defs := DefaultRoleDefinitions()
	for _, def := range defs {
		for _, rid := range def.RuleIDs {
			if strings.EqualFold(strings.TrimSpace(rid), RuleCommunicationUpstream) {
				t.Fatalf("role %q should not include %q; upstream communication is runtime-contextual", def.Name, RuleCommunicationUpstream)
			}
		}
	}
}

func TestDefaultRoleDefinitions_IdentityIsTopLevel(t *testing.T) {
	defs := DefaultRoleDefinitions()
	for _, def := range defs {
		if strings.TrimSpace(def.Identity) == "" {
			t.Fatalf("role %q should include top-level identity text", def.Name)
		}
	}
}

func TestEnsureDefaultRoleCatalog_StripsLegacyUpstreamCommunicationRule(t *testing.T) {
	cfg := &GlobalConfig{
		PromptRules: []PromptRule{
			{ID: RuleCommunicationUpstream, Body: "legacy upstream rule"},
			{ID: RuleDeveloperIdentity, Body: "dev rule"},
		},
		Roles: []RoleDefinition{
			{
				Name:         "developer",
				CanWriteCode: true,
				RuleIDs:      []string{RuleDeveloperIdentity, RuleCommunicationUpstream},
			},
		},
		DefaultRole: "developer",
	}

	changed := EnsureDefaultRoleCatalog(cfg)
	if !changed {
		t.Fatalf("EnsureDefaultRoleCatalog should report changes when stripping legacy upstream rule")
	}

	if cfg.FindPromptRule(RuleCommunicationUpstream) != nil {
		t.Fatalf("legacy upstream prompt rule should be removed from catalog")
	}
	if cfg.FindPromptRule(RuleDeveloperIdentity) != nil {
		t.Fatalf("legacy identity prompt rule should be removed from catalog")
	}

	role := cfg.FindRoleDefinition("developer")
	if role == nil {
		t.Fatalf("developer role missing after normalization")
	}
	for _, rid := range role.RuleIDs {
		if strings.EqualFold(strings.TrimSpace(rid), RuleCommunicationUpstream) {
			t.Fatalf("developer role still contains stripped upstream rule: %v", role.RuleIDs)
		}
		if strings.EqualFold(strings.TrimSpace(rid), RuleDeveloperIdentity) {
			t.Fatalf("developer role still contains stripped identity rule: %v", role.RuleIDs)
		}
	}
	if got := strings.TrimSpace(role.Identity); got != "dev rule" {
		t.Fatalf("developer identity = %q, want %q", got, "dev rule")
	}
}
