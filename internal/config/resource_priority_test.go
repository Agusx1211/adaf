package config

import "testing"

func TestNormalizeResourcePriority(t *testing.T) {
	if got := NormalizeResourcePriority("  QUALITY "); got != ResourcePriorityQuality {
		t.Fatalf("NormalizeResourcePriority() = %q, want %q", got, ResourcePriorityQuality)
	}
}

func TestValidResourcePriority(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "quality", want: true},
		{value: "normal", want: true},
		{value: "cost", want: true},
		{value: "QUALITY", want: true},
		{value: "  cost  ", want: true},
		{value: "fast", want: false},
		{value: "", want: false},
	}

	for _, tt := range tests {
		if got := ValidResourcePriority(tt.value); got != tt.want {
			t.Fatalf("ValidResourcePriority(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestEffectiveResourcePriority_DefaultsToNormal(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: ResourcePriorityNormal},
		{value: "   ", want: ResourcePriorityNormal},
		{value: "quality", want: ResourcePriorityQuality},
		{value: "QUALITY", want: ResourcePriorityQuality},
		{value: "cost", want: ResourcePriorityCost},
		{value: "unknown", want: ResourcePriorityNormal},
	}

	for _, tt := range tests {
		if got := EffectiveResourcePriority(tt.value); got != tt.want {
			t.Fatalf("EffectiveResourcePriority(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestParseResourcePriority(t *testing.T) {
	got, err := ParseResourcePriority(" quality ")
	if err != nil {
		t.Fatalf("ParseResourcePriority() error = %v, want nil", err)
	}
	if got != ResourcePriorityQuality {
		t.Fatalf("ParseResourcePriority() = %q, want %q", got, ResourcePriorityQuality)
	}

	got, err = ParseResourcePriority("")
	if err != nil {
		t.Fatalf("ParseResourcePriority(empty) error = %v, want nil", err)
	}
	if got != ResourcePriorityNormal {
		t.Fatalf("ParseResourcePriority(empty) = %q, want %q", got, ResourcePriorityNormal)
	}

	if _, err := ParseResourcePriority("fast"); err == nil {
		t.Fatal("ParseResourcePriority(invalid) error = nil, want non-nil")
	}
}
