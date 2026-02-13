package config

import "testing"

func TestEffectiveStepRole(t *testing.T) {
	tests := []struct {
		name     string
		stepRole string
		want     string
	}{
		{
			name:     "uses explicit step role",
			stepRole: RoleManager,
			want:     RoleManager,
		},
		{
			name:     "defaults to junior with empty values",
			stepRole: "",
			want:     RoleJunior,
		},
		{
			name:     "invalid values default to junior",
			stepRole: "invalid",
			want:     RoleJunior,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveStepRole(tt.stepRole)
			if got != tt.want {
				t.Fatalf("EffectiveStepRole() = %q, want %q", got, tt.want)
			}
		})
	}
}
