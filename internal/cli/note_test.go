package cli

import "testing"

func TestResolveNoteTurnID(t *testing.T) {
	t.Run("uses explicit turn flag", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "42")
		got, err := resolveRequiredTurnID(7)
		if err != nil {
			t.Fatalf("resolveRequiredTurnID returned error: %v", err)
		}
		if got != 7 {
			t.Fatalf("turn id = %d, want 7", got)
		}
	})

	t.Run("uses ADAF_TURN_ID when flag is omitted", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "42")
		got, err := resolveRequiredTurnID(0)
		if err != nil {
			t.Fatalf("resolveRequiredTurnID returned error: %v", err)
		}
		if got != 42 {
			t.Fatalf("turn id = %d, want 42", got)
		}
	})

	t.Run("falls back to ADAF_SESSION_ID for backward compat", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "99")
		got, err := resolveRequiredTurnID(0)
		if err != nil {
			t.Fatalf("resolveRequiredTurnID returned error: %v", err)
		}
		if got != 99 {
			t.Fatalf("turn id = %d, want 99", got)
		}
	})

	t.Run("errors when omitted and env missing", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "")
		_, err := resolveRequiredTurnID(0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("errors on invalid ADAF_TURN_ID", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "abc")
		_, err := resolveRequiredTurnID(0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestResolveOptionalTurnID(t *testing.T) {
	t.Run("returns zero when omitted and env missing", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "")
		got, err := resolveOptionalTurnID(0)
		if err != nil {
			t.Fatalf("resolveOptionalTurnID returned error: %v", err)
		}
		if got != 0 {
			t.Fatalf("turn id = %d, want 0", got)
		}
	})

	t.Run("uses ADAF_TURN_ID env when available", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "15")
		got, err := resolveOptionalTurnID(0)
		if err != nil {
			t.Fatalf("resolveOptionalTurnID returned error: %v", err)
		}
		if got != 15 {
			t.Fatalf("turn id = %d, want 15", got)
		}
	})

	t.Run("falls back to ADAF_SESSION_ID", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "25")
		got, err := resolveOptionalTurnID(0)
		if err != nil {
			t.Fatalf("resolveOptionalTurnID returned error: %v", err)
		}
		if got != 25 {
			t.Fatalf("turn id = %d, want 25", got)
		}
	})
}
