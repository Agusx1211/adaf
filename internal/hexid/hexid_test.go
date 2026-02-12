package hexid

import (
	"regexp"
	"testing"
)

func TestNew(t *testing.T) {
	id := New()
	if len(id) != 8 {
		t.Fatalf("expected length 8, got %d: %q", len(id), id)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(id) {
		t.Fatalf("expected lowercase hex, got %q", id)
	}
}

func TestNewUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := New()
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate ID after %d iterations: %q", i, id)
		}
		seen[id] = struct{}{}
	}
}
