package config

import (
	"fmt"
	"strings"
)

// Built-in execution positions.
const (
	PositionSupervisor = "supervisor"
	PositionManager    = "manager"
	PositionLead       = "lead"
	PositionWorker     = "worker"
)

func normalizePositionName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// AllPositions returns the built-in positions in display order.
func AllPositions() []string {
	return []string{
		PositionSupervisor,
		PositionManager,
		PositionLead,
		PositionWorker,
	}
}

// ValidPosition reports whether the value is one of the built-in positions.
func ValidPosition(position string) bool {
	switch normalizePositionName(position) {
	case PositionSupervisor, PositionManager, PositionLead, PositionWorker:
		return true
	default:
		return false
	}
}

// EffectiveStepPosition resolves the effective position for a loop step.
func EffectiveStepPosition(step LoopStep) string {
	if pos := normalizePositionName(step.Position); ValidPosition(pos) {
		return pos
	}
	return PositionLead
}

// DefaultWorkerRole resolves the fallback worker role.
func DefaultWorkerRole(cfg ...*GlobalConfig) string {
	role := normalizeRoleName(DefaultRole(cfg...))
	if role != "" && ValidRole(role, cfg...) {
		return role
	}
	return RoleDeveloper
}

// EffectiveWorkerRoleForPosition resolves a concrete worker role for prompts
// and runtime checks. Non-worker positions return empty role.
func EffectiveWorkerRoleForPosition(position, role string, cfg ...*GlobalConfig) string {
	if normalizePositionName(position) != PositionWorker {
		return ""
	}
	role = normalizeRoleName(role)
	if role == "" || !ValidRole(role, cfg...) {
		return DefaultWorkerRole(cfg...)
	}
	return role
}

// PositionCanWriteCode reports whether the position can directly modify code.
func PositionCanWriteCode(position string) bool {
	switch normalizePositionName(position) {
	case PositionLead, PositionWorker:
		return true
	default:
		return false
	}
}

// CanWriteForPositionAndRole resolves write capability from position first,
// with worker-role capability refinement.
func CanWriteForPositionAndRole(position, role string, cfg ...*GlobalConfig) bool {
	pos := normalizePositionName(position)
	switch pos {
	case PositionSupervisor, PositionManager:
		return false
	case PositionLead:
		return true
	case PositionWorker:
		return CanWriteCode(EffectiveWorkerRoleForPosition(pos, role, cfg...), cfg...)
	default:
		return CanWriteCode(role, cfg...)
	}
}

// PositionRequiresTeam reports whether a position must have a team assigned.
func PositionRequiresTeam(position string) bool {
	return normalizePositionName(position) == PositionManager
}

// PositionAllowsTeam reports whether a position is allowed to have a team.
func PositionAllowsTeam(position string) bool {
	switch normalizePositionName(position) {
	case PositionManager, PositionLead:
		return true
	default:
		return false
	}
}

// PositionCanSpawn reports whether a position is allowed to spawn sub-agents.
func PositionCanSpawn(position string) bool {
	switch normalizePositionName(position) {
	case PositionManager, PositionLead:
		return true
	default:
		return false
	}
}

// PositionCanStopLoop reports whether the position can signal a loop stop.
// This is intentionally supervisor-only.
func PositionCanStopLoop(position string) bool {
	return normalizePositionName(position) == PositionSupervisor
}

// PositionCanMessageLoop reports whether the position can post guidance to
// subsequent loop steps via `adaf loop message`.
func PositionCanMessageLoop(position string) bool {
	return normalizePositionName(position) == PositionSupervisor
}

// PositionCanCallSupervisor reports whether the position can escalate to the
// supervisor via `adaf loop call-supervisor`.
func PositionCanCallSupervisor(position string) bool {
	return normalizePositionName(position) == PositionManager
}

// PositionCanOwnTurn reports whether this position can be assigned as a loop step owner.
func PositionCanOwnTurn(position string) bool {
	return normalizePositionName(position) != PositionWorker
}

// PositionMustWriteTurnLog reports whether turn-handoff logging is mandatory.
func PositionMustWriteTurnLog(position string) bool {
	switch normalizePositionName(position) {
	case PositionSupervisor, PositionManager:
		return true
	default:
		return false
	}
}

// ValidateDelegationForPosition validates a delegation tree against the
// position model. Teams are intentionally flat and composed only of workers.
func ValidateDelegationForPosition(deleg *DelegationConfig) error {
	if deleg == nil {
		return nil
	}
	for _, dp := range deleg.Profiles {
		pos := dp.EffectivePosition()
		if pos != PositionWorker {
			return fmt.Errorf("delegation profile %q must use worker position (got %q)", dp.Name, pos)
		}
		roles, err := dp.EffectiveRoles()
		if err != nil {
			return fmt.Errorf("delegation profile %q roles: %w", dp.Name, err)
		}
		if len(roles) == 0 {
			return fmt.Errorf("delegation profile %q must define at least one worker role", dp.Name)
		}
		if dp.Delegation != nil && len(dp.Delegation.Profiles) > 0 {
			return fmt.Errorf("delegation profile %q cannot define child delegation; workers cannot have teams", dp.Name)
		}
	}
	return nil
}

// ValidateLoopStepPosition validates one loop step against position constraints.
func ValidateLoopStepPosition(step LoopStep, cfg *GlobalConfig) error {
	pos := EffectiveStepPosition(step)
	if !PositionCanOwnTurn(pos) {
		return fmt.Errorf("loop step profile %q uses position %q; workers can only be spawned as sub-agents", step.Profile, pos)
	}

	hasTeam := strings.TrimSpace(step.Team) != ""
	if PositionRequiresTeam(pos) && !hasTeam {
		return fmt.Errorf("loop step profile %q uses position %q and requires a team", step.Profile, pos)
	}
	if !PositionAllowsTeam(pos) && hasTeam {
		return fmt.Errorf("loop step profile %q uses position %q and cannot have a team", step.Profile, pos)
	}
	if !hasTeam {
		return nil
	}
	if cfg == nil {
		return fmt.Errorf("loop step profile %q requires global config to resolve team %q", step.Profile, step.Team)
	}
	team := cfg.FindTeam(step.Team)
	if team == nil {
		return fmt.Errorf("loop step profile %q references unknown team %q", step.Profile, step.Team)
	}
	if PositionRequiresTeam(pos) && (team.Delegation == nil || len(team.Delegation.Profiles) == 0) {
		return fmt.Errorf("loop step profile %q uses position %q and requires a non-empty team", step.Profile, pos)
	}
	if err := ValidateDelegationForPosition(team.Delegation); err != nil {
		return fmt.Errorf("team %q: %w", team.Name, err)
	}
	return nil
}
