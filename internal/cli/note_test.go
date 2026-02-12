package cli

import "testing"

func TestResolveNoteSessionID(t *testing.T) {
	t.Run("uses explicit session flag", func(t *testing.T) {
		t.Setenv("ADAF_SESSION_ID", "42")
		got, err := resolveRequiredSessionID(7)
		if err != nil {
			t.Fatalf("resolveRequiredSessionID returned error: %v", err)
		}
		if got != 7 {
			t.Fatalf("session id = %d, want 7", got)
		}
	})

	t.Run("uses ADAF_SESSION_ID when flag is omitted", func(t *testing.T) {
		t.Setenv("ADAF_SESSION_ID", "42")
		got, err := resolveRequiredSessionID(0)
		if err != nil {
			t.Fatalf("resolveRequiredSessionID returned error: %v", err)
		}
		if got != 42 {
			t.Fatalf("session id = %d, want 42", got)
		}
	})

	t.Run("errors when omitted and env missing", func(t *testing.T) {
		t.Setenv("ADAF_SESSION_ID", "")
		_, err := resolveRequiredSessionID(0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("errors on invalid ADAF_SESSION_ID", func(t *testing.T) {
		t.Setenv("ADAF_SESSION_ID", "abc")
		_, err := resolveRequiredSessionID(0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestResolveOptionalSessionID(t *testing.T) {
	t.Run("returns zero when omitted and env missing", func(t *testing.T) {
		t.Setenv("ADAF_SESSION_ID", "")
		got, err := resolveOptionalSessionID(0)
		if err != nil {
			t.Fatalf("resolveOptionalSessionID returned error: %v", err)
		}
		if got != 0 {
			t.Fatalf("session id = %d, want 0", got)
		}
	})

	t.Run("uses env when available", func(t *testing.T) {
		t.Setenv("ADAF_SESSION_ID", "15")
		got, err := resolveOptionalSessionID(0)
		if err != nil {
			t.Fatalf("resolveOptionalSessionID returned error: %v", err)
		}
		if got != 15 {
			t.Fatalf("session id = %d, want 15", got)
		}
	})
}
