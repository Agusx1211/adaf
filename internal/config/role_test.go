package config

import "testing"

func TestEffectiveStepRole(t *testing.T) {
	tests := []struct {
		name     string
		stepRole string
		want     string
	}{
		{
			name:     "uses explicit worker role",
			stepRole: RoleQA,
			want:     RoleQA,
		},
		{
			name:     "defaults to developer with empty values",
			stepRole: "",
			want:     RoleDeveloper,
		},
		{
			name:     "invalid values default to developer",
			stepRole: "invalid",
			want:     RoleDeveloper,
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
