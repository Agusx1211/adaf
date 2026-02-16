package store

import (
	"testing"
)

func TestUpdateChatInstanceLastSession(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	inst, err := s.CreateChatInstance("test-profile", "", nil)
	if err != nil {
		t.Fatalf("CreateChatInstance: %v", err)
	}

	if inst.LastSessionID != 0 {
		t.Fatalf("initial LastSessionID = %d, want 0", inst.LastSessionID)
	}

	if err := s.UpdateChatInstanceLastSession(inst.ID, 77); err != nil {
		t.Fatalf("UpdateChatInstanceLastSession: %v", err)
	}

	got, err := s.GetChatInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetChatInstance: %v", err)
	}
	if got.LastSessionID != 77 {
		t.Fatalf("LastSessionID = %d, want 77", got.LastSessionID)
	}

	// Overwrite with new session ID.
	if err := s.UpdateChatInstanceLastSession(inst.ID, 88); err != nil {
		t.Fatalf("UpdateChatInstanceLastSession(88): %v", err)
	}
	got, err = s.GetChatInstance(inst.ID)
	if err != nil {
		t.Fatalf("GetChatInstance: %v", err)
	}
	if got.LastSessionID != 88 {
		t.Fatalf("LastSessionID = %d, want 88", got.LastSessionID)
	}
}
