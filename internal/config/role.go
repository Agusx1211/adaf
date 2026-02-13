package config

// Role constants.
const (
	RoleManager       = "manager"
	RoleLeadDeveloper = "lead-developer"
	RoleDeveloper     = "developer"
	RoleSupervisor    = "supervisor"

	RoleUIDesigner      = "ui-designer"
	RoleQA              = "qa"
	RoleBackendDesigner = "backend-designer"
	RoleDocumentator    = "documentator"
	RoleReviewer        = "reviewer"
	RoleScout           = "scout"
	RoleResearcher      = "researcher"
)

func firstRoleCfg(cfg ...*GlobalConfig) *GlobalConfig {
	for _, c := range cfg {
		if c != nil {
			return c
		}
	}
	return nil
}

func roleCatalog(cfg *GlobalConfig) []RoleDefinition {
	if cfg == nil {
		return DefaultRoleDefinitions()
	}
	EnsureDefaultRoleCatalog(cfg)
	return cfg.Roles
}

// ValidRole reports whether role is a recognised role string.
func ValidRole(role string, cfg ...*GlobalConfig) bool {
	role = normalizeRoleName(role)
	if role == "" {
		return true
	}
	for _, def := range roleCatalog(firstRoleCfg(cfg...)) {
		if normalizeRoleName(def.Name) == role {
			return true
		}
	}
	return false
}

// DefaultRole returns the configured default role, falling back to developer.
func DefaultRole(cfg ...*GlobalConfig) string {
	c := firstRoleCfg(cfg...)
	if c != nil {
		EnsureDefaultRoleCatalog(c)
		if ValidRole(c.DefaultRole, c) {
			return normalizeRoleName(c.DefaultRole)
		}
	}
	if ValidRole(RoleDeveloper, c) {
		return RoleDeveloper
	}
	roles := AllRoles(c)
	if len(roles) > 0 {
		return roles[0]
	}
	return RoleDeveloper
}

// EffectiveRole returns the normalized role value: empty defaults to configured default role.
func EffectiveRole(role string, cfg ...*GlobalConfig) string {
	role = normalizeRoleName(role)
	if role == "" {
		return DefaultRole(cfg...)
	}
	if ValidRole(role, cfg...) {
		return role
	}
	return DefaultRole(cfg...)
}

// EffectiveStepRole resolves the role for a loop step.
//
// Priority:
//  1. Explicit step role
//  2. configured default role
func EffectiveStepRole(stepRole string, cfg ...*GlobalConfig) string {
	return EffectiveRole(stepRole, cfg...)
}

// CanWriteCode reports whether the given role is allowed to modify files.
func CanWriteCode(role string, cfg ...*GlobalConfig) bool {
	effective := EffectiveRole(role, cfg...)
	for _, def := range roleCatalog(firstRoleCfg(cfg...)) {
		if normalizeRoleName(def.Name) == effective {
			return def.CanWriteCode
		}
	}
	// Unknown roles should not be write-blocked by default.
	return true
}

// AllRoles returns all valid role strings in display order.
func AllRoles(cfg ...*GlobalConfig) []string {
	defs := roleCatalog(firstRoleCfg(cfg...))
	out := make([]string, 0, len(defs))
	seen := make(map[string]struct{}, len(defs))
	for _, def := range defs {
		name := normalizeRoleName(def.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return []string{RoleDeveloper}
	}
	return out
}
