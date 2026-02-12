package config

// DelegationProfile describes one profile available for spawning.
type DelegationProfile struct {
	Name         string `json:"name"`                    // profile name
	MaxInstances int    `json:"max_instances,omitempty"` // max concurrent (0 = unlimited)
	Speed        string `json:"speed,omitempty"`         // "fast", "medium", "slow" — informational
	Handoff      bool   `json:"handoff,omitempty"`       // can be transferred to next loop step
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

// AllStylePresets returns all valid style preset names.
func AllStylePresets() []string {
	return []string{StylePresetManager, StylePresetParallel, StylePresetScout, StylePresetSequential}
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

// HasProfile checks whether a profile name is in the delegation's profile list.
func (d *DelegationConfig) HasProfile(name string) bool {
	if d == nil {
		return false
	}
	for _, p := range d.Profiles {
		if p.Name == name {
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
		if d.Profiles[i].Name == name {
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
