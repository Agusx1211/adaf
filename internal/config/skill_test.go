package config

import "testing"

func TestDefaultSkills(t *testing.T) {
	skills := DefaultSkills()
	if len(skills) != 12 {
		t.Fatalf("DefaultSkills() returned %d skills, want 12", len(skills))
	}

	// Verify all built-in skill IDs are present.
	wantIDs := []string{
		SkillAutonomy, SkillCodeWriting, SkillCommit, SkillFocus,
		SkillAdafTools, SkillDelegation, SkillIssues, SkillPlan,
		SkillSessionContext, SkillLoopControl, SkillReadOnly, SkillPushover,
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

func TestEnsureDefaultSkillCatalog_Seeds(t *testing.T) {
	cfg := &GlobalConfig{}
	changed := EnsureDefaultSkillCatalog(cfg)
	if !changed {
		t.Fatal("EnsureDefaultSkillCatalog should return true when seeding defaults")
	}
	if len(cfg.Skills) != 12 {
		t.Fatalf("seeded %d skills, want 12", len(cfg.Skills))
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

	sk := cfg.FindSkill("autonomy")
	if sk == nil {
		t.Fatal("FindSkill(autonomy) = nil")
	}
	if sk.ID != SkillAutonomy {
		t.Fatalf("FindSkill(autonomy).ID = %q, want %q", sk.ID, SkillAutonomy)
	}

	sk = cfg.FindSkill("AUTONOMY")
	if sk == nil {
		t.Fatal("FindSkill(AUTONOMY) = nil, case-insensitive lookup failed")
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
