package config

import "testing"

func TestDelegationProfileEffectiveRoles(t *testing.T) {
	tests := []struct {
		name    string
		prof    DelegationProfile
		want    []string
		wantErr bool
	}{
		{
			name: "explicit single role",
			prof: DelegationProfile{
				Name: "worker",
				Role: RoleDeveloper,
			},
			want: []string{RoleDeveloper},
		},
		{
			name: "multiple explicit roles",
			prof: DelegationProfile{
				Name:  "worker",
				Roles: []string{RoleDeveloper, RoleQA, RoleDeveloper},
			},
			want: []string{RoleDeveloper, RoleQA},
		},
		{
			name: "falls back to developer",
			prof: DelegationProfile{
				Name: "worker",
			},
			want: []string{RoleDeveloper},
		},
		{
			name: "non worker position has no roles",
			prof: DelegationProfile{
				Name:     "reviewer",
				Position: PositionManager,
			},
			want: nil,
		},
		{
			name: "custom explicit role is accepted",
			prof: DelegationProfile{
				Name: "worker",
				Role: "bad",
			},
			want: []string{"bad"},
		},
		{
			name: "custom role in list is accepted",
			prof: DelegationProfile{
				Name:  "worker",
				Roles: []string{RoleDeveloper, "bad"},
			},
			want: []string{RoleDeveloper, "bad"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.prof.EffectiveRoles()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("EffectiveRoles() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("EffectiveRoles() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("EffectiveRoles() len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("EffectiveRoles()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDelegationResolveProfile(t *testing.T) {
	d := &DelegationConfig{
		Profiles: []DelegationProfile{
			{Name: "worker", Role: RoleDeveloper},
			{Name: "worker", Role: RoleQA},
			{Name: "manager", Position: PositionManager},
			{Name: "analyst", Roles: []string{RoleDeveloper, RoleQA}},
			{Name: "unspecified"}, // no role set
		},
	}

	t.Run("selects explicit role entry", func(t *testing.T) {
		dp, role, pos, err := d.ResolveProfileWithPosition("worker", RoleQA, "")
		if err != nil {
			t.Fatalf("ResolveProfileWithPosition() error = %v", err)
		}
		if role != RoleQA {
			t.Fatalf("role = %q, want %q", role, RoleQA)
		}
		if pos != PositionWorker {
			t.Fatalf("position = %q, want %q", pos, PositionWorker)
		}
		if dp == nil {
			t.Fatal("resolved profile is nil")
		}
		if dp.Role != RoleQA {
			t.Fatalf("resolved profile role = %q, want %q", dp.Role, RoleQA)
		}
	})

	t.Run("requires role when ambiguous", func(t *testing.T) {
		_, _, _, err := d.ResolveProfileWithPosition("worker", "", "")
		if err == nil {
			t.Fatalf("ResolveProfileWithPosition() error = nil, want ambiguity error")
		}
	})

	t.Run("defaults to developer when role is missing", func(t *testing.T) {
		dp, role, pos, err := d.ResolveProfileWithPosition("unspecified", "", "")
		if err != nil {
			t.Fatalf("ResolveProfileWithPosition() error = %v", err)
		}
		if role != RoleDeveloper {
			t.Fatalf("role = %q, want %q", role, RoleDeveloper)
		}
		if pos != PositionWorker {
			t.Fatalf("position = %q, want %q", pos, PositionWorker)
		}
		if dp == nil || dp.Role != RoleDeveloper {
			t.Fatalf("resolved role = %v, want %q", dp, RoleDeveloper)
		}
	})

	t.Run("supports multi-role entry with explicit role", func(t *testing.T) {
		dp, role, pos, err := d.ResolveProfileWithPosition("analyst", RoleDeveloper, "")
		if err != nil {
			t.Fatalf("ResolveProfileWithPosition() error = %v", err)
		}
		if role != RoleDeveloper {
			t.Fatalf("role = %q, want %q", role, RoleDeveloper)
		}
		if pos != PositionWorker {
			t.Fatalf("position = %q, want %q", pos, PositionWorker)
		}
		if dp == nil || dp.Role != RoleDeveloper {
			t.Fatalf("resolved role = %v, want %q", dp, RoleDeveloper)
		}
		if len(dp.Roles) != 0 {
			t.Fatalf("resolved profile roles should be normalized to single role, got %v", dp.Roles)
		}
	})

	t.Run("errors for invalid requested role", func(t *testing.T) {
		_, _, _, err := d.ResolveProfileWithPosition("analyst", "bad", "")
		if err == nil {
			t.Fatalf("ResolveProfileWithPosition() error = nil, want error")
		}
	})

	t.Run("resolves non-worker position when explicit", func(t *testing.T) {
		dp, role, pos, err := d.ResolveProfileWithPosition("manager", "", PositionManager)
		if err != nil {
			t.Fatalf("ResolveProfileWithPosition() error = %v", err)
		}
		if role != "" {
			t.Fatalf("role = %q, want empty", role)
		}
		if pos != PositionManager {
			t.Fatalf("position = %q, want %q", pos, PositionManager)
		}
		if dp == nil || dp.Position != PositionManager {
			t.Fatalf("resolved profile position = %v, want %q", dp, PositionManager)
		}
	})
}

func TestDelegationCloneDeepCopy(t *testing.T) {
	orig := &DelegationConfig{
		MaxParallel: 3,
		StylePreset: StylePresetParallel,
		Profiles: []DelegationProfile{
			{
				Name:  "worker",
				Roles: []string{RoleDeveloper, RoleQA},
				Delegation: &DelegationConfig{
					Profiles: []DelegationProfile{{Name: "scout", Role: RoleDeveloper}},
				},
			},
		},
	}

	cloned := orig.Clone()
	if cloned == nil {
		t.Fatal("Clone() = nil")
	}

	cloned.MaxParallel = 99
	cloned.Profiles[0].Roles[0] = RoleQA
	cloned.Profiles[0].Delegation.Profiles[0].Name = "changed"

	if orig.MaxParallel != 3 {
		t.Fatalf("orig.MaxParallel changed to %d", orig.MaxParallel)
	}
	if orig.Profiles[0].Roles[0] != RoleDeveloper {
		t.Fatalf("orig roles mutated: %v", orig.Profiles[0].Roles)
	}
	if orig.Profiles[0].Delegation.Profiles[0].Name != "scout" {
		t.Fatalf("orig nested delegation mutated: %v", orig.Profiles[0].Delegation.Profiles[0].Name)
	}
}

func TestCollectDelegationProfileNames(t *testing.T) {
	d := &DelegationConfig{
		Profiles: []DelegationProfile{
			{
				Name: "A",
				Delegation: &DelegationConfig{
					Profiles: []DelegationProfile{
						{Name: "B"},
						{
							Name: "C",
							Delegation: &DelegationConfig{
								Profiles: []DelegationProfile{{Name: "D"}},
							},
						},
					},
				},
			},
			{Name: "b"}, // duplicate (case-insensitive)
			{Name: "E"},
		},
	}

	got := CollectDelegationProfileNames(d)
	want := []string{"A", "B", "C", "D", "E"}
	if len(got) != len(want) {
		t.Fatalf("CollectDelegationProfileNames() len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("CollectDelegationProfileNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
