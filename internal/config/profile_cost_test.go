package config

import "testing"

func TestNormalizeProfileCost(t *testing.T) {
	got := NormalizeProfileCost("  ChEaP ")
	if got != ProfileCostCheap {
		t.Fatalf("NormalizeProfileCost() = %q, want %q", got, ProfileCostCheap)
	}
}

func TestValidProfileCost(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "free", value: "free", want: true},
		{name: "cheap uppercase", value: "CHEAP", want: true},
		{name: "normal spaces", value: " normal ", want: true},
		{name: "expensive", value: "expensive", want: true},
		{name: "invalid", value: "premium", want: false},
		{name: "empty", value: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidProfileCost(tt.value); got != tt.want {
				t.Fatalf("ValidProfileCost(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestAllowedProfileCosts(t *testing.T) {
	got := AllowedProfileCosts()
	want := []string{ProfileCostFree, ProfileCostCheap, ProfileCostNormal, ProfileCostExpensive}
	if len(got) != len(want) {
		t.Fatalf("AllowedProfileCosts() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllowedProfileCosts()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
