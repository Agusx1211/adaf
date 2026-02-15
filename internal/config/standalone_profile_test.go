package config

import "testing"

func TestStandaloneProfileCRUD(t *testing.T) {
	cfg := &GlobalConfig{}

	// Add
	sp := StandaloneProfile{Name: "my-sp", Profile: "worker", Instructions: "do stuff"}
	if err := cfg.AddStandaloneProfile(sp); err != nil {
		t.Fatalf("AddStandaloneProfile() error = %v", err)
	}
	if len(cfg.StandaloneProfiles) != 1 {
		t.Fatalf("expected 1 standalone profile, got %d", len(cfg.StandaloneProfiles))
	}

	// Duplicate check (case-insensitive)
	dup := StandaloneProfile{Name: "MY-SP", Profile: "worker"}
	if err := cfg.AddStandaloneProfile(dup); err == nil {
		t.Fatalf("AddStandaloneProfile() expected duplicate error, got nil")
	}

	// Find (case-insensitive)
	found := cfg.FindStandaloneProfile("My-Sp")
	if found == nil {
		t.Fatalf("FindStandaloneProfile() returned nil")
	}
	if found.Name != "my-sp" {
		t.Fatalf("FindStandaloneProfile().Name = %q, want %q", found.Name, "my-sp")
	}
	if found.Instructions != "do stuff" {
		t.Fatalf("FindStandaloneProfile().Instructions = %q, want %q", found.Instructions, "do stuff")
	}

	// Find missing
	if cfg.FindStandaloneProfile("nonexistent") != nil {
		t.Fatalf("FindStandaloneProfile() expected nil for nonexistent")
	}

	// Add another
	sp2 := StandaloneProfile{Name: "other-sp", Profile: "analyst"}
	if err := cfg.AddStandaloneProfile(sp2); err != nil {
		t.Fatalf("AddStandaloneProfile() error = %v", err)
	}
	if len(cfg.StandaloneProfiles) != 2 {
		t.Fatalf("expected 2 standalone profiles, got %d", len(cfg.StandaloneProfiles))
	}

	// Remove (case-insensitive)
	cfg.RemoveStandaloneProfile("MY-SP")
	if len(cfg.StandaloneProfiles) != 1 {
		t.Fatalf("expected 1 standalone profile after remove, got %d", len(cfg.StandaloneProfiles))
	}
	if cfg.StandaloneProfiles[0].Name != "other-sp" {
		t.Fatalf("remaining profile = %q, want %q", cfg.StandaloneProfiles[0].Name, "other-sp")
	}

	// Remove nonexistent (no-op)
	cfg.RemoveStandaloneProfile("nonexistent")
	if len(cfg.StandaloneProfiles) != 1 {
		t.Fatalf("expected 1 standalone profile after noop remove, got %d", len(cfg.StandaloneProfiles))
	}
}
