package config

import (
	"strings"
	"testing"
)

func TestDefaultSkills(t *testing.T) {
	skills := DefaultSkills()
	if len(skills) != 2 {
		t.Fatalf("DefaultSkills() returned %d skills, want 2", len(skills))
	}

	// Verify only delegation + pushover are built in.
	wantIDs := []string{
		SkillDelegation, SkillPushover,
	}
	seen := make(map[string]struct{}, len(skills))
	for _, sk := range skills {
		seen[sk.ID] = struct{}{}
	}
	for _, id := range wantIDs {
		if _, ok := seen[id]; !ok {
			t.Errorf("missing skill %q in DefaultSkills()", id)
		}
	}

	// Every skill should have a non-empty Short.
	for _, sk := range skills {
		if sk.Short == "" {
			t.Errorf("skill %q has empty Short", sk.ID)
		}
	}

	// Every skill should have a non-empty Long.
	for _, sk := range skills {
		if sk.Long == "" {
			t.Errorf("skill %q has empty Long", sk.ID)
		}
	}
}

func TestDelegationSkillMentionsSpawnInfo(t *testing.T) {
	skills := DefaultSkills()
	var deleg *Skill
	for i := range skills {
		if skills[i].ID == SkillDelegation {
			deleg = &skills[i]
			break
		}
	}
	if deleg == nil {
		t.Fatalf("missing %q skill", SkillDelegation)
	}
	if !containsFold(deleg.Short, "adaf spawn-info") {
		t.Fatalf("delegation short text should mention adaf spawn-info, got: %q", deleg.Short)
	}
	if !containsFold(deleg.Long, "adaf wait-for-spawns") {
		t.Fatalf("delegation long text should mention adaf wait-for-spawns")
	}
}

func TestEnsureDefaultSkillCatalog_Seeds(t *testing.T) {
	cfg := &GlobalConfig{}
	changed := EnsureDefaultSkillCatalog(cfg)
	if !changed {
		t.Fatal("EnsureDefaultSkillCatalog should return true when seeding defaults")
	}
	if len(cfg.Skills) != 2 {
		t.Fatalf("seeded %d skills, want 2", len(cfg.Skills))
	}
}

func TestEnsureDefaultSkillCatalog_Normalizes(t *testing.T) {
	cfg := &GlobalConfig{
		Skills: []Skill{
			{ID: "  AUTONOMY ", Short: "test"},
			{ID: "autonomy", Short: "dupe"},
			{ID: "", Short: "empty"},
			{ID: "custom", Short: "custom skill"},
		},
	}
	changed := EnsureDefaultSkillCatalog(cfg)
	if !changed {
		t.Fatal("EnsureDefaultSkillCatalog should return true when normalizing")
	}
	if len(cfg.Skills) != 2 {
		t.Fatalf("got %d skills, want 2 (autonomy + custom)", len(cfg.Skills))
	}
	if cfg.Skills[0].ID != "autonomy" {
		t.Fatalf("first skill ID = %q, want %q", cfg.Skills[0].ID, "autonomy")
	}
	if cfg.Skills[1].ID != "custom" {
		t.Fatalf("second skill ID = %q, want %q", cfg.Skills[1].ID, "custom")
	}
}

func TestEnsureDefaultSkillCatalog_NoChange(t *testing.T) {
	cfg := &GlobalConfig{
		Skills: []Skill{
			{ID: "autonomy", Short: "test"},
		},
	}
	changed := EnsureDefaultSkillCatalog(cfg)
	if changed {
		t.Fatal("EnsureDefaultSkillCatalog should return false when no changes needed")
	}
}

func TestEnsureDefaultSkillCatalog_Nil(t *testing.T) {
	changed := EnsureDefaultSkillCatalog(nil)
	if changed {
		t.Fatal("EnsureDefaultSkillCatalog(nil) should return false")
	}
}

func TestFindSkill(t *testing.T) {
	cfg := &GlobalConfig{}
	EnsureDefaultSkillCatalog(cfg)

	sk := cfg.FindSkill("delegation")
	if sk == nil {
		t.Fatal("FindSkill(delegation) = nil")
	}
	if sk.ID != SkillDelegation {
		t.Fatalf("FindSkill(delegation).ID = %q, want %q", sk.ID, SkillDelegation)
	}

	sk = cfg.FindSkill("DELEGATION")
	if sk == nil {
		t.Fatal("FindSkill(DELEGATION) = nil, case-insensitive lookup failed")
	}

	sk = cfg.FindSkill("nonexistent")
	if sk != nil {
		t.Fatal("FindSkill(nonexistent) should be nil")
	}
}

func TestAddSkill(t *testing.T) {
	cfg := &GlobalConfig{
		Skills: []Skill{{ID: "existing", Short: "test"}},
	}

	err := cfg.AddSkill(Skill{ID: "new_skill", Short: "new"})
	if err != nil {
		t.Fatalf("AddSkill: %v", err)
	}
	if len(cfg.Skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(cfg.Skills))
	}

	// Duplicate should fail.
	err = cfg.AddSkill(Skill{ID: "existing", Short: "dup"})
	if err == nil {
		t.Fatal("AddSkill(existing) should fail")
	}

	// Empty ID should fail.
	err = cfg.AddSkill(Skill{ID: "", Short: "empty"})
	if err == nil {
		t.Fatal("AddSkill(empty) should fail")
	}
}

func TestRemoveSkill(t *testing.T) {
	cfg := &GlobalConfig{
		Skills: []Skill{
			{ID: "a", Short: "A"},
			{ID: "b", Short: "B"},
			{ID: "c", Short: "C"},
		},
	}

	cfg.RemoveSkill("b")
	if len(cfg.Skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(cfg.Skills))
	}
	if cfg.FindSkill("b") != nil {
		t.Fatal("skill b should be removed")
	}
	if cfg.FindSkill("a") == nil {
		t.Fatal("skill a should still exist")
	}
	if cfg.FindSkill("c") == nil {
		t.Fatal("skill c should still exist")
	}
}

func TestResolveSkillsForContext(t *testing.T) {
	cfg := &GlobalConfig{}
	EnsureDefaultRoleCatalog(cfg)
	EnsureDefaultSkillCatalog(cfg)

	allSkills := []string{
		SkillAutonomy, SkillCodeWriting, SkillCommit, SkillFocus,
		SkillAdafTools, SkillDelegation,
	}

	tests := []struct {
		name     string
		skills   []string
		position string
		role     string
		readOnly bool
		wantHas  []string // skills that should be present
		wantNot  []string // skills that should be absent
	}{
		{
			name:     "writing role keeps code_writing and commit",
			skills:   allSkills,
			position: PositionLead,
			role:     RoleDeveloper,
			wantHas:  []string{SkillCodeWriting, SkillCommit, SkillAutonomy},
			wantNot:  []string{SkillCodeReview, SkillReadOnly},
		},
		{
			name:     "non-writing role replaces code_writing with code_review",
			skills:   allSkills,
			position: PositionManager,
			role:     RoleDeveloper,
			wantHas:  []string{SkillCodeReview, SkillAutonomy},
			wantNot:  []string{SkillCodeWriting, SkillCommit},
		},
		{
			name:     "read-only mode removes commit and adds read_only",
			skills:   allSkills,
			position: PositionLead,
			role:     RoleDeveloper,
			readOnly: true,
			wantHas:  []string{SkillCodeWriting, SkillReadOnly, SkillAutonomy},
			wantNot:  []string{SkillCommit},
		},
		{
			name:     "non-writing role + read-only",
			skills:   allSkills,
			position: PositionManager,
			role:     RoleDeveloper,
			readOnly: true,
			wantHas:  []string{SkillCodeReview, SkillReadOnly},
			wantNot:  []string{SkillCodeWriting, SkillCommit},
		},
		{
			name:     "read-only does not duplicate read_only if already present",
			skills:   []string{SkillReadOnly, SkillAutonomy},
			position: PositionLead,
			role:     RoleDeveloper,
			readOnly: true,
			wantHas:  []string{SkillReadOnly, SkillAutonomy},
		},
		{
			name:     "empty skills returns empty",
			skills:   []string{},
			position: PositionLead,
			role:     RoleDeveloper,
			wantHas:  []string{},
			wantNot:  []string{SkillCodeWriting, SkillCommit},
		},
		{
			name:     "empty skills + read-only adds read_only",
			skills:   []string{},
			position: PositionLead,
			role:     RoleDeveloper,
			readOnly: true,
			wantHas:  []string{SkillReadOnly},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSkillsForContext(tt.skills, tt.position, tt.role, tt.readOnly, cfg)

			has := func(id string) bool {
				for _, s := range result {
					if s == id {
						return true
					}
				}
				return false
			}

			for _, id := range tt.wantHas {
				if !has(id) {
					t.Errorf("expected skill %q in result %v", id, result)
				}
			}
			for _, id := range tt.wantNot {
				if has(id) {
					t.Errorf("unexpected skill %q in result %v", id, result)
				}
			}

			// Verify input was not mutated.
			originalCopy := make([]string, len(tt.skills))
			copy(originalCopy, tt.skills)
			ResolveSkillsForContext(tt.skills, tt.position, tt.role, tt.readOnly, cfg)
			for i := range tt.skills {
				if tt.skills[i] != originalCopy[i] {
					t.Errorf("input slice was mutated at index %d: got %q, want %q", i, tt.skills[i], originalCopy[i])
				}
			}
		})
	}
}

func TestEffectiveStepSkills(t *testing.T) {
	tests := []struct {
		name string
		step LoopStep
		want []string
	}{
		{
			name: "default nil skills",
			step: LoopStep{},
			want: nil,
		},
		{
			name: "legacy explicit list",
			step: LoopStep{Skills: []string{SkillAutonomy, SkillFocus}},
			want: []string{SkillAutonomy, SkillFocus},
		},
		{
			name: "explicit empty skills disables defaults",
			step: LoopStep{SkillsExplicit: true},
			want: []string{},
		},
		{
			name: "explicit empty non-nil skills disables defaults",
			step: LoopStep{SkillsExplicit: true, Skills: []string{}},
			want: []string{},
		},
		{
			name: "explicit custom skills",
			step: LoopStep{SkillsExplicit: true, Skills: []string{SkillPlan}},
			want: []string{SkillPlan},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveStepSkills(tt.step)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("EffectiveStepSkills() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("EffectiveStepSkills() = nil, want %v", tt.want)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("EffectiveStepSkills() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("EffectiveStepSkills()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
