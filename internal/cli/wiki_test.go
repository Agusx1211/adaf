package cli

import (
	"testing"
)

func TestWikiTitleFromID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"testing-conventions", "Testing Conventions"},
		{"api_design", "Api Design"},
		{"simple", "Simple"},
		{"multi-word-slug-here", "Multi Word Slug Here"},
		{"mixed_and-styles", "Mixed And Styles"},
		{"ALLCAPS", "ALLCAPS"},
		{"a", "A"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := wikiTitleFromID(tt.id)
			if got != tt.want {
				t.Errorf("wikiTitleFromID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
