package session

import (
	"os"
	"testing"
)

func TestWriteAndReadAgentSessionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a session directory.
	sessionID := 999
	if err := os.MkdirAll(SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write and read back.
	if err := WriteAgentSessionID(sessionID, "claude-sess-abc123"); err != nil {
		t.Fatalf("WriteAgentSessionID: %v", err)
	}
	got := ReadAgentSessionID(sessionID)
	if got != "claude-sess-abc123" {
		t.Fatalf("ReadAgentSessionID = %q, want %q", got, "claude-sess-abc123")
	}
}

func TestReadAgentSessionID_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := ReadAgentSessionID(12345)
	if got != "" {
		t.Fatalf("ReadAgentSessionID(missing) = %q, want empty", got)
	}
}

func TestWriteAgentSessionID_Overwrites(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionID := 800
	if err := os.MkdirAll(SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := WriteAgentSessionID(sessionID, "first"); err != nil {
		t.Fatalf("WriteAgentSessionID(first): %v", err)
	}
	if err := WriteAgentSessionID(sessionID, "second"); err != nil {
		t.Fatalf("WriteAgentSessionID(second): %v", err)
	}

	got := ReadAgentSessionID(sessionID)
	if got != "second" {
		t.Fatalf("ReadAgentSessionID = %q, want %q", got, "second")
	}
}
