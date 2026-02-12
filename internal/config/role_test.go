package config

import "testing"

func TestEffectiveStepRole(t *testing.T) {
	tests := []struct {
		name     string
		stepRole string
		profile  *Profile
		want     string
	}{
		{
			name:     "uses explicit step role",
			stepRole: RoleManager,
			profile:  &Profile{Role: RoleJunior},
			want:     RoleManager,
		},
		{
			name:     "falls back to profile role",
			stepRole: "",
			profile:  &Profile{Role: RoleSenior},
			want:     RoleSenior,
		},
		{
			name:     "defaults to junior with empty values",
			stepRole: "",
			profile:  &Profile{},
			want:     RoleJunior,
		},
		{
			name:     "defaults to junior with nil profile",
			stepRole: "",
			profile:  nil,
			want:     RoleJunior,
		},
		{
			name:     "invalid step role falls back to profile role",
			stepRole: "invalid",
			profile:  &Profile{Role: RoleManager},
			want:     RoleManager,
		},
		{
			name:     "invalid values default to junior",
			stepRole: "invalid",
			profile:  &Profile{Role: "also-invalid"},
			want:     RoleJunior,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveStepRole(tt.stepRole, tt.profile)
			if got != tt.want {
				t.Fatalf("EffectiveStepRole() = %q, want %q", got, tt.want)
			}
		})
	}
}
