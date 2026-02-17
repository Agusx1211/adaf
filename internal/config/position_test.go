package config

import "testing"

func TestEffectiveStepPosition(t *testing.T) {
	tests := []struct {
		name string
		step LoopStep
		want string
	}{
		{
			name: "explicit position wins",
			step: LoopStep{Position: PositionSupervisor, Role: RoleDeveloper},
			want: PositionSupervisor,
		},
		{
			name: "role does not change top-level position",
			step: LoopStep{Role: RoleDeveloper},
			want: PositionLead,
		},
		{
			name: "empty defaults to lead",
			step: LoopStep{},
			want: PositionLead,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveStepPosition(tt.step)
			if got != tt.want {
				t.Fatalf("EffectiveStepPosition() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateLoopStepPosition(t *testing.T) {
	cfg := &GlobalConfig{
		Teams: []Team{
			{
				Name: "workers",
				Delegation: &DelegationConfig{
					Profiles: []DelegationProfile{
						{Name: "dev-a", Position: PositionWorker, Role: RoleDeveloper},
					},
				},
			},
			{
				Name:       "empty",
				Delegation: &DelegationConfig{},
			},
		},
	}

	tests := []struct {
		name    string
		step    LoopStep
		wantErr bool
	}{
		{
			name:    "worker cannot own loop step",
			step:    LoopStep{Profile: "p", Position: PositionWorker},
			wantErr: true,
		},
		{
			name:    "supervisor cannot have team",
			step:    LoopStep{Profile: "p", Position: PositionSupervisor, Team: "workers"},
			wantErr: true,
		},
		{
			name:    "manager must have team",
			step:    LoopStep{Profile: "p", Position: PositionManager},
			wantErr: true,
		},
		{
			name:    "manager requires non-empty team",
			step:    LoopStep{Profile: "p", Position: PositionManager, Team: "empty"},
			wantErr: true,
		},
		{
			name:    "lead may have no team",
			step:    LoopStep{Profile: "p", Position: PositionLead},
			wantErr: false,
		},
		{
			name:    "manager with worker team is valid",
			step:    LoopStep{Profile: "p", Position: PositionManager, Team: "workers"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLoopStepPosition(tt.step, cfg)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateLoopStepPosition() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateLoopStepPosition() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateDelegationForPosition(t *testing.T) {
	t.Run("rejects non worker profile", func(t *testing.T) {
		err := ValidateDelegationForPosition(&DelegationConfig{
			Profiles: []DelegationProfile{
				{Name: "mgr", Position: PositionManager},
			},
		})
		if err == nil {
			t.Fatalf("ValidateDelegationForPosition() error = nil, want error")
		}
	})

	t.Run("rejects nested worker delegation", func(t *testing.T) {
		err := ValidateDelegationForPosition(&DelegationConfig{
			Profiles: []DelegationProfile{
				{
					Name:     "dev",
					Position: PositionWorker,
					Role:     RoleDeveloper,
					Delegation: &DelegationConfig{
						Profiles: []DelegationProfile{
							{Name: "nested", Position: PositionWorker, Role: RoleDeveloper},
						},
					},
				},
			},
		})
		if err == nil {
			t.Fatalf("ValidateDelegationForPosition() error = nil, want error")
		}
	})
}
