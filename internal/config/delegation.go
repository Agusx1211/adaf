package config

import (
	"fmt"
	"sort"
	"strings"
)

// DelegationProfile describes one profile available for spawning.
type DelegationProfile struct {
	Name           string            `json:"name"`                      // profile name
	Position       string            `json:"position,omitempty"`        // execution level for this spawned profile
	Role           string            `json:"role,omitempty"`            // explicit role for this spawn option
	Roles          []string          `json:"roles,omitempty"`           // allowed roles for this spawn option
	MaxInstances   int               `json:"max_instances,omitempty"`   // max concurrent (0 = unlimited)
	TimeoutMinutes int               `json:"timeout_minutes,omitempty"` // max runtime per spawn (0 = unlimited)
	Speed          string            `json:"speed,omitempty"`           // "fast", "medium", "slow" — informational
	Handoff        bool              `json:"handoff,omitempty"`         // can be transferred to next loop step
	Delegation     *DelegationConfig `json:"delegation,omitempty"`      // child spawn rules for this option
	Skills         []string          `json:"skills,omitempty"`          // skill IDs for spawned agents
}

// DelegationConfig describes spawn capabilities for a loop step or session.
type DelegationConfig struct {
	Profiles    []DelegationProfile `json:"profiles"`               // which profiles can be spawned
	MaxParallel int                 `json:"max_parallel,omitempty"` // total concurrent spawns (0 = default 4)
	Style       string              `json:"style,omitempty"`        // free-form delegation style guidance
	StylePreset string              `json:"style_preset,omitempty"` // preset name (overrides style if set)
}

// Style preset constants.
const (
	StylePresetManager    = "manager"
	StylePresetParallel   = "parallel"
	StylePresetScout      = "scout"
	StylePresetSequential = "sequential"
)

// stylePresetTexts maps preset names to prompt text.
var stylePresetTexts = map[string]string{
	StylePresetManager: "Do NOT write code. Break down the task, spawn sub-agents for all implementation, " +
		"review diffs, merge or reject.",
	StylePresetParallel: "Write code yourself AND spawn sub-agents for independent sub-tasks concurrently.",
	StylePresetScout: "Spawn read-only agents for research and investigation. " +
		"Use their findings to guide your own work.",
	StylePresetSequential: "Spawn one sub-agent at a time. Wait for completion before spawning the next.",
}

// StylePresetText returns the prompt text for a style preset name.
// Returns empty string for unknown presets.
func StylePresetText(preset string) string {
	return stylePresetTexts[preset]
}

// DelegationStyleText returns the effective style text for a delegation config.
// If StylePreset is set, it takes precedence over Style.
func (d *DelegationConfig) DelegationStyleText() string {
	if d == nil {
		return ""
	}
	if d.StylePreset != "" {
		if text := StylePresetText(d.StylePreset); text != "" {
			return text
		}
	}
	return d.Style
}

// Clone deep-copies a DelegationConfig tree.
func (d *DelegationConfig) Clone() *DelegationConfig {
	if d == nil {
		return nil
	}
	out := &DelegationConfig{
		MaxParallel: d.MaxParallel,
		Style:       d.Style,
		StylePreset: d.StylePreset,
	}
	if len(d.Profiles) > 0 {
		out.Profiles = make([]DelegationProfile, len(d.Profiles))
		for i := range d.Profiles {
			p := d.Profiles[i]
			if len(p.Roles) > 0 {
				p.Roles = append([]string(nil), p.Roles...)
			}
			if len(p.Skills) > 0 {
				p.Skills = append([]string(nil), p.Skills...)
			}
			p.Delegation = p.Delegation.Clone()
			out.Profiles[i] = p
		}
	}
	return out
}

// HasProfile checks whether a profile name is in the delegation's profile list.
func (d *DelegationConfig) HasProfile(name string) bool {
	if d == nil {
		return false
	}
	for _, p := range d.Profiles {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}

// FindProfile returns the DelegationProfile for a given name, or nil.
func (d *DelegationConfig) FindProfile(name string) *DelegationProfile {
	if d == nil {
		return nil
	}
	for i := range d.Profiles {
		if strings.EqualFold(d.Profiles[i].Name, name) {
			return &d.Profiles[i]
		}
	}
	return nil
}

// EffectiveMaxParallel returns the max parallel value, defaulting to 4.
func (d *DelegationConfig) EffectiveMaxParallel() int {
	if d == nil || d.MaxParallel <= 0 {
		return 4
	}
	return d.MaxParallel
}

func normalizeRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

// EffectivePosition resolves the effective position for this spawn option.
//
// Priority:
//  1. Position (explicit built-in position)
//  2. worker
func (p *DelegationProfile) EffectivePosition() string {
	if p == nil {
		return PositionWorker
	}
	if pos := normalizePositionName(p.Position); ValidPosition(pos) {
		return pos
	}
	return PositionWorker
}

// EffectiveRoles resolves allowed roles for this spawn option.
//
// Priority:
//  1. Role (single explicit role)
//  2. Roles (multiple explicit roles)
//  3. "developer"
//
// For non-worker positions, this returns no roles.
func (p *DelegationProfile) EffectiveRoles() ([]string, error) {
	if p == nil {
		return nil, nil
	}
	if p.EffectivePosition() != PositionWorker {
		return nil, nil
	}
	if role := normalizeRole(p.Role); role != "" {
		return []string{role}, nil
	}
	if len(p.Roles) > 0 {
		roles := make([]string, 0, len(p.Roles))
		seen := make(map[string]struct{}, len(p.Roles))
		for _, raw := range p.Roles {
			role := normalizeRole(raw)
			if role == "" {
				continue
			}
			if _, ok := seen[role]; ok {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
		if len(roles) > 0 {
			return roles, nil
		}
	}
	return []string{DefaultWorkerRole()}, nil
}

type resolvedDelegationProfile struct {
	index    int
	roles    []string
	position string
}

func roleInList(role string, roles []string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func sortedRoleKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for role := range m {
		out = append(out, role)
	}
	order := map[string]int{
		RoleDeveloper:  0,
		RoleQA:         1,
		RoleScout:      2,
		RoleResearcher: 3,
	}
	sort.Slice(out, func(i, j int) bool {
		li, lok := order[out[i]]
		rj, rok := order[out[j]]
		switch {
		case lok && rok:
			return li < rj
		case lok:
			return true
		case rok:
			return false
		default:
			return out[i] < out[j]
		}
	})
	return out
}

func pickCandidate(candidates []resolvedDelegationProfile, role string, position string) (resolvedDelegationProfile, bool) {
	filtered := make([]resolvedDelegationProfile, 0, len(candidates))
	for _, c := range candidates {
		if position == "" || c.position == position {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		return resolvedDelegationProfile{}, false
	}
	if position != PositionWorker {
		return filtered[0], true
	}
	// Prefer explicit single-role matches first.
	for _, c := range filtered {
		if len(c.roles) == 1 && c.roles[0] == role {
			return c, true
		}
	}
	for _, c := range filtered {
		if roleInList(role, c.roles) {
			return c, true
		}
	}
	return resolvedDelegationProfile{}, false
}

// ResolveProfileWithPosition selects a single spawn option by child profile,
// role, and position.
//
// requestedRole may be empty. When empty and multiple roles are available for
// the same profile, an explicit role is required.
func (d *DelegationConfig) ResolveProfileWithPosition(name, requestedRole, requestedPosition string) (*DelegationProfile, string, string, error) {
	if d == nil {
		return nil, "", "", fmt.Errorf("delegation config is nil")
	}
	requestedRole = normalizeRole(requestedRole)
	requestedPosition = normalizePositionName(requestedPosition)
	if requestedPosition != "" && !ValidPosition(requestedPosition) {
		return nil, "", "", fmt.Errorf("invalid requested position %q", requestedPosition)
	}

	candidates := make([]resolvedDelegationProfile, 0, len(d.Profiles))
	allRoles := make(map[string]struct{})
	allPositions := make(map[string]struct{})
	for i := range d.Profiles {
		p := d.Profiles[i]
		if !strings.EqualFold(p.Name, name) {
			continue
		}
		position := p.EffectivePosition()
		if requestedPosition != "" && requestedPosition != position {
			continue
		}
		roles, err := p.EffectiveRoles()
		if err != nil {
			return nil, "", "", err
		}
		if position == PositionWorker && len(roles) == 0 {
			continue
		}
		if position == PositionWorker && requestedRole != "" && !roleInList(requestedRole, roles) {
			continue
		}
		if position != PositionWorker && requestedRole != "" {
			continue
		}
		candidates = append(candidates, resolvedDelegationProfile{
			index:    i,
			roles:    roles,
			position: position,
		})
		allPositions[position] = struct{}{}
		if position == PositionWorker {
			for _, role := range roles {
				allRoles[role] = struct{}{}
			}
		}
	}

	if len(candidates) == 0 {
		if requestedRole != "" {
			return nil, "", "", fmt.Errorf("profile %q cannot be spawned as role %q", name, requestedRole)
		}
		if requestedPosition != "" {
			return nil, "", "", fmt.Errorf("profile %q cannot be spawned as position %q", name, requestedPosition)
		}
		return nil, "", "", fmt.Errorf("profile %q is not in delegation profiles", name)
	}

	position := requestedPosition
	if position == "" {
		if len(allPositions) == 1 {
			for p := range allPositions {
				position = p
			}
		} else {
			keys := make([]string, 0, len(allPositions))
			for p := range allPositions {
				keys = append(keys, p)
			}
			sort.Strings(keys)
			return nil, "", "", fmt.Errorf("profile %q has invalid delegation with mixed positions (%s); teams must define worker-only spawn entries",
				name, strings.Join(keys, ", "))
		}
	}

	role := requestedRole
	if position == PositionWorker {
		if role == "" {
			roleKeys := sortedRoleKeys(allRoles)
			if len(roleKeys) == 1 {
				role = roleKeys[0]
			} else {
				return nil, "", "", fmt.Errorf("profile %q can be spawned with multiple roles (%s); specify --role",
					name, strings.Join(roleKeys, ", "))
			}
		}
	} else {
		role = ""
	}

	match, ok := pickCandidate(candidates, role, position)
	if !ok {
		if position != PositionWorker {
			return nil, "", "", fmt.Errorf("profile %q cannot be spawned as position %q", name, position)
		}
		return nil, "", "", fmt.Errorf("profile %q cannot be spawned as role %q", name, role)
	}

	resolved := d.Profiles[match.index]
	resolved.Position = position
	resolved.Role = role
	resolved.Roles = nil
	resolved.Delegation = resolved.Delegation.Clone()
	return &resolved, role, position, nil
}

// CollectDelegationProfileNames returns all unique profile names present in a
// delegation tree (depth-first traversal order).
func CollectDelegationProfileNames(deleg *DelegationConfig) []string {
	if deleg == nil {
		return nil
	}
	seenCfg := make(map[*DelegationConfig]struct{})
	seenNames := make(map[string]struct{})
	var names []string

	var walk func(cur *DelegationConfig)
	walk = func(cur *DelegationConfig) {
		if cur == nil {
			return
		}
		if _, ok := seenCfg[cur]; ok {
			return
		}
		seenCfg[cur] = struct{}{}
		for _, dp := range cur.Profiles {
			name := strings.TrimSpace(dp.Name)
			if name != "" {
				key := strings.ToLower(name)
				if _, ok := seenNames[key]; !ok {
					seenNames[key] = struct{}{}
					names = append(names, name)
				}
			}
			walk(dp.Delegation)
		}
	}

	walk(deleg)
	return names
}
