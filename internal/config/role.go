package config

// Role constants.
const (
	RoleManager    = "manager"
	RoleSenior     = "senior"
	RoleJunior     = "junior"
	RoleSupervisor = "supervisor"
)

// validRoles is the set of recognised role values.
var validRoles = map[string]bool{
	RoleManager:    true,
	RoleSenior:     true,
	RoleJunior:     true,
	RoleSupervisor: true,
}

// ValidRole reports whether role is a recognised role string.
func ValidRole(role string) bool {
	return role == "" || validRoles[role]
}

// EffectiveRole returns the role to use for a profile: empty defaults to "junior".
func EffectiveRole(role string) string {
	if role == "" {
		return RoleJunior
	}
	return role
}

// EffectiveStepRole resolves the role for a loop step.
//
// Priority:
//  1. Explicit step role
//  2. Legacy profile role (if profile != nil)
//  3. "junior"
func EffectiveStepRole(stepRole string, profile *Profile) string {
	if stepRole != "" && ValidRole(stepRole) {
		return stepRole
	}
	if profile != nil && profile.Role != "" && ValidRole(profile.Role) {
		return profile.Role
	}
	return RoleJunior
}

// CanSpawn reports whether the given role is traditionally allowed to spawn sub-agents.
// Deprecated: spawning is now controlled by DelegationConfig on loop steps / session starts.
// This is kept for backward compatibility with profile creation UIs.
func CanSpawn(role string) bool {
	r := EffectiveRole(role)
	return r == RoleManager || r == RoleSenior
}

// CanWriteCode reports whether the given role is allowed to modify files.
func CanWriteCode(role string) bool {
	r := EffectiveRole(role)
	return r == RoleSenior || r == RoleJunior
}

// AllRoles returns all valid role strings in display order.
func AllRoles() []string {
	return []string{RoleManager, RoleSenior, RoleJunior, RoleSupervisor}
}
