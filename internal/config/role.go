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

// CanSpawn reports whether the given role is allowed to spawn sub-agents.
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
