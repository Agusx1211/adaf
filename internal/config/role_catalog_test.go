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

func TestDefaultRoleDefinitions_ExcludeLegacyPositionRoles(t *testing.T) {
	defs := DefaultRoleDefinitions()
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		seen[def.Name] = struct{}{}
	}
	for _, legacy := range []string{"manager", "supervisor", "lead-developer"} {
		if _, ok := seen[legacy]; ok {
			t.Fatalf("default role catalog should not include legacy position-like role %q", legacy)
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

func TestEnsureDefaultRoleCatalog_StripsReservedPositionRoles(t *testing.T) {
	cfg := &GlobalConfig{
		Roles: []RoleDefinition{
			{Name: "manager", Title: "MANAGER"},
			{Name: "lead-developer", Title: "LEAD"},
			{Name: "supervisor", Title: "SUPERVISOR"},
			{Name: "developer", Title: "DEVELOPER"},
		},
		DefaultRole: "manager",
	}

	changed := EnsureDefaultRoleCatalog(cfg)
	if !changed {
		t.Fatalf("EnsureDefaultRoleCatalog() changed = false, want true")
	}
	if len(cfg.Roles) != 1 || cfg.Roles[0].Name != "developer" {
		t.Fatalf("roles after normalization = %#v, want only developer", cfg.Roles)
	}
	if cfg.DefaultRole != "developer" {
		t.Fatalf("default role = %q, want %q", cfg.DefaultRole, "developer")
	}
}

func TestAddRoleDefinition_RejectsReservedPositionNames(t *testing.T) {
	cfg := &GlobalConfig{}
	EnsureDefaultRoleCatalog(cfg)

	for _, name := range []string{"manager", "supervisor", "lead", "worker", "lead-developer"} {
		if err := cfg.AddRoleDefinition(RoleDefinition{Name: name}); err == nil {
			t.Fatalf("AddRoleDefinition(%q) error = nil, want reserved-name error", name)
		}
	}
}
