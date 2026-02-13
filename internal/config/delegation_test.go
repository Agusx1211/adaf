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
				Role: RoleLeadDeveloper,
			},
			want: []string{RoleLeadDeveloper},
		},
		{
			name: "multiple explicit roles",
			prof: DelegationProfile{
				Name:  "worker",
				Roles: []string{RoleDeveloper, RoleLeadDeveloper, RoleDeveloper},
			},
			want: []string{RoleDeveloper, RoleLeadDeveloper},
		},
		{
			name: "falls back to developer",
			prof: DelegationProfile{
				Name: "worker",
			},
			want: []string{RoleDeveloper},
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
			{Name: "worker", Role: RoleLeadDeveloper, Delegation: &DelegationConfig{Profiles: []DelegationProfile{{Name: "scout", Role: RoleDeveloper}}}},
			{Name: "analyst", Roles: []string{RoleDeveloper, RoleLeadDeveloper}},
			{Name: "unspecified"}, // no role set
		},
	}

	t.Run("selects explicit role entry", func(t *testing.T) {
		dp, role, err := d.ResolveProfile("worker", RoleLeadDeveloper)
		if err != nil {
			t.Fatalf("ResolveProfile() error = %v", err)
		}
		if role != RoleLeadDeveloper {
			t.Fatalf("role = %q, want %q", role, RoleLeadDeveloper)
		}
		if dp == nil {
			t.Fatal("resolved profile is nil")
		}
		if dp.Role != RoleLeadDeveloper {
			t.Fatalf("resolved profile role = %q, want %q", dp.Role, RoleLeadDeveloper)
		}
		if dp.Delegation == nil || !dp.Delegation.HasProfile("scout") {
			t.Fatalf("resolved profile delegation missing expected child rules")
		}
	})

	t.Run("requires role when ambiguous", func(t *testing.T) {
		_, _, err := d.ResolveProfile("worker", "")
		if err == nil {
			t.Fatalf("ResolveProfile() error = nil, want ambiguity error")
		}
	})

	t.Run("defaults to developer when role is missing", func(t *testing.T) {
		dp, role, err := d.ResolveProfile("unspecified", "")
		if err != nil {
			t.Fatalf("ResolveProfile() error = %v", err)
		}
		if role != RoleDeveloper {
			t.Fatalf("role = %q, want %q", role, RoleDeveloper)
		}
		if dp == nil || dp.Role != RoleDeveloper {
			t.Fatalf("resolved role = %v, want %q", dp, RoleDeveloper)
		}
	})

	t.Run("supports multi-role entry with explicit role", func(t *testing.T) {
		dp, role, err := d.ResolveProfile("analyst", RoleDeveloper)
		if err != nil {
			t.Fatalf("ResolveProfile() error = %v", err)
		}
		if role != RoleDeveloper {
			t.Fatalf("role = %q, want %q", role, RoleDeveloper)
		}
		if dp == nil || dp.Role != RoleDeveloper {
			t.Fatalf("resolved role = %v, want %q", dp, RoleDeveloper)
		}
		if len(dp.Roles) != 0 {
			t.Fatalf("resolved profile roles should be normalized to single role, got %v", dp.Roles)
		}
	})

	t.Run("errors for invalid requested role", func(t *testing.T) {
		_, _, err := d.ResolveProfile("analyst", "bad")
		if err == nil {
			t.Fatalf("ResolveProfile() error = nil, want error")
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
				Roles: []string{RoleDeveloper, RoleLeadDeveloper},
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
	cloned.Profiles[0].Roles[0] = RoleManager
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
