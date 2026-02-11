package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Profile is a named agent+model combo stored in the global config.
type Profile struct {
	Name          string `json:"name"`                      // Display name, e.g. "codex 5.2"
	Agent         string `json:"agent"`                     // Agent key: "claude", "codex", etc.
	Model         string `json:"model,omitempty"`           // Model override (empty = agent default)
	ReasoningLevel string `json:"reasoning_level,omitempty"` // Reasoning level (e.g. "low", "medium", "high", "xhigh")

	// Orchestration fields (all optional — existing profiles work unchanged).
	Role              string   `json:"role,omitempty"`               // "manager", "senior", "junior", "supervisor" (empty = "junior")
	MaxParallel       int      `json:"max_parallel,omitempty"`       // max concurrent sub-agents (manager/senior)
	SpawnableProfiles []string `json:"spawnable_profiles,omitempty"` // profile names this can spawn
	Description       string   `json:"description,omitempty"`        // strengths/weaknesses text
	Intelligence      int      `json:"intelligence,omitempty"`       // 1-10 capability rating
	MaxInstances      int      `json:"max_instances,omitempty"`      // max concurrent instances of this profile (0 = unlimited)
}

// GlobalConfig holds user-level preferences stored in ~/.adaf/config.json.
type GlobalConfig struct {
	Agents   map[string]GlobalAgentConfig `json:"agents,omitempty"`
	Profiles []Profile                    `json:"profiles,omitempty"`
}

// GlobalAgentConfig holds per-agent overrides at the global (user) level.
type GlobalAgentConfig struct {
	ModelOverride string `json:"model_override,omitempty"`
	Path          string `json:"path,omitempty"`
}

// Dir returns the global adaf config directory (~/.adaf), creating it if needed.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".adaf")
	os.MkdirAll(dir, 0755)
	return dir
}

// configPath returns the full path to ~/.adaf/config.json.
func configPath() string {
	return filepath.Join(Dir(), "config.json")
}

// Load reads ~/.adaf/config.json, returning an empty config if the file is absent.
func Load() (*GlobalConfig, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &GlobalConfig{Agents: make(map[string]GlobalAgentConfig)}, nil
		}
		return nil, err
	}

	var cfg GlobalConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]GlobalAgentConfig)
	}
	return &cfg, nil
}

// Save writes the global config to ~/.adaf/config.json.
func Save(cfg *GlobalConfig) error {
	if cfg == nil {
		cfg = &GlobalConfig{}
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]GlobalAgentConfig)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}

// AddProfile appends a profile. Returns an error if the name already exists.
func (c *GlobalConfig) AddProfile(p Profile) error {
	for _, existing := range c.Profiles {
		if strings.EqualFold(existing.Name, p.Name) {
			return errors.New("profile already exists: " + p.Name)
		}
	}
	c.Profiles = append(c.Profiles, p)
	return nil
}

// RemoveProfile removes a profile by name (case-insensitive).
func (c *GlobalConfig) RemoveProfile(name string) {
	out := c.Profiles[:0]
	for _, p := range c.Profiles {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	c.Profiles = out
}

// FindProfile returns a pointer to a profile by name, or nil if not found.
func (c *GlobalConfig) FindProfile(name string) *Profile {
	for i := range c.Profiles {
		if strings.EqualFold(c.Profiles[i].Name, name) {
			return &c.Profiles[i]
		}
	}
	return nil
}
