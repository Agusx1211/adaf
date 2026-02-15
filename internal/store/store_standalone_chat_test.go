package store

import (
	"testing"
)

func TestStandaloneChatLastSession_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.WriteStandaloneChatLastSession("myprofile", 42); err != nil {
		t.Fatalf("WriteStandaloneChatLastSession: %v", err)
	}

	got := s.ReadStandaloneChatLastSession("myprofile")
	if got != 42 {
		t.Fatalf("ReadStandaloneChatLastSession = %d, want 42", got)
	}
}

func TestStandaloneChatLastSession_MissingReturnsZero(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got := s.ReadStandaloneChatLastSession("nonexistent")
	if got != 0 {
		t.Fatalf("ReadStandaloneChatLastSession(missing) = %d, want 0", got)
	}
}

func TestStandaloneChatLastSession_Overwrites(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.WriteStandaloneChatLastSession("prof", 10); err != nil {
		t.Fatalf("WriteStandaloneChatLastSession(10): %v", err)
	}
	if err := s.WriteStandaloneChatLastSession("prof", 20); err != nil {
		t.Fatalf("WriteStandaloneChatLastSession(20): %v", err)
	}

	got := s.ReadStandaloneChatLastSession("prof")
	if got != 20 {
		t.Fatalf("ReadStandaloneChatLastSession = %d, want 20", got)
	}
}

func TestClearStandaloneChatMessages_AlsoClearsLastSession(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Write a message and a last session ID.
	msg := &StandaloneChatMessage{Role: "user", Content: "hello"}
	if err := s.CreateStandaloneChatMessage("prof", msg); err != nil {
		t.Fatalf("CreateStandaloneChatMessage: %v", err)
	}
	if err := s.WriteStandaloneChatLastSession("prof", 99); err != nil {
		t.Fatalf("WriteStandaloneChatLastSession: %v", err)
	}

	// Clear should remove both messages and last_session_id.
	if err := s.ClearStandaloneChatMessages("prof"); err != nil {
		t.Fatalf("ClearStandaloneChatMessages: %v", err)
	}

	msgs, err := s.ListStandaloneChatMessages("prof")
	if err != nil {
		t.Fatalf("ListStandaloneChatMessages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages after clear = %d, want 0", len(msgs))
	}

	got := s.ReadStandaloneChatLastSession("prof")
	if got != 0 {
		t.Fatalf("ReadStandaloneChatLastSession after clear = %d, want 0", got)
	}
}

func TestUpdateChatInstanceLastSession(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	inst, err := s.CreateChatInstance("test-profile")
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
