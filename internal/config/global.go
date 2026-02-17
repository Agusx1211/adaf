package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Profile is a named agent+model combo stored in the global config.
type Profile struct {
	Name           string `json:"name"`                      // Display name, e.g. "codex 5.2"
	Agent          string `json:"agent"`                     // Agent key: "claude", "codex", etc.
	Model          string `json:"model,omitempty"`           // Model override (empty = agent default)
	ReasoningLevel string `json:"reasoning_level,omitempty"` // Reasoning level (e.g. "low", "medium", "high", "xhigh")

	// Profile metadata fields.
	Description  string `json:"description,omitempty"`   // strengths/weaknesses text
	Intelligence int    `json:"intelligence,omitempty"`  // 1-10 capability rating
	MaxInstances int    `json:"max_instances,omitempty"` // max concurrent instances of this profile (0 = unlimited)
	Speed        string `json:"speed,omitempty"`         // "fast", "medium", "slow" — informational speed rating
}

// LoopStep defines one step in a loop cycle.
type LoopStep struct {
	Profile        string   `json:"profile"`                   // profile name reference
	Role           string   `json:"role,omitempty"`            // role name from global roles catalog
	Turns          int      `json:"turns,omitempty"`           // turns per step (0 = 1 turn)
	Instructions   string   `json:"instructions,omitempty"`    // custom instructions appended to prompt
	CanStop        bool     `json:"can_stop,omitempty"`        // can this step signal loop stop?
	CanMessage     bool     `json:"can_message,omitempty"`     // can this step send messages to subsequent steps?
	CanPushover    bool     `json:"can_pushover,omitempty"`    // can this step send Pushover notifications?
	Team           string   `json:"team,omitempty"`            // team name reference (resolved to delegation at runtime)
	StandaloneChat bool     `json:"standalone_chat,omitempty"` // interactive chat mode (minimal prompt)
	Skills         []string `json:"skills,omitempty"`          // skill IDs to activate for this step
}

// LoopDef defines a loop as a cyclic template of profile steps.
type LoopDef struct {
	Name  string     `json:"name"`
	Steps []LoopStep `json:"steps"`
}

// PushoverConfig holds Pushover notification credentials.
type PushoverConfig struct {
	UserKey  string `json:"user_key,omitempty"`  // Pushover user/group key
	AppToken string `json:"app_token,omitempty"` // Pushover application API token
}

// Team is a named, reusable DelegationConfig.
type Team struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Delegation  *DelegationConfig `json:"delegation,omitempty"`
}

// RecentCombination tracks a recently used profile+team pair for quick-pick in the UI.
type RecentCombination struct {
	Profile string    `json:"profile"`
	Team    string    `json:"team"`
	UsedAt  time.Time `json:"used_at"`
}

// RecentProject tracks a recently opened project for persistence across web server restarts.
type RecentProject struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	RootDir  string    `json:"root_dir"`
	OpenedAt time.Time `json:"opened_at"`
}

// GlobalConfig holds user-level preferences stored in ~/.adaf/config.json.
type GlobalConfig struct {
	Agents             map[string]GlobalAgentConfig `json:"agents,omitempty"`
	Profiles           []Profile                    `json:"profiles,omitempty"`
	Loops              []LoopDef                    `json:"loops,omitempty"`
	Teams              []Team                       `json:"teams,omitempty"`
	RecentCombinations []RecentCombination          `json:"recent_combinations,omitempty"`
	RecentProjects     []RecentProject               `json:"recent_projects,omitempty"`
	Pushover           PushoverConfig               `json:"pushover,omitempty"`
	PromptRules        []PromptRule                 `json:"prompt_rules,omitempty"`
	Roles              []RoleDefinition             `json:"roles,omitempty"`
	DefaultRole        string                       `json:"default_role,omitempty"`
	Skills             []Skill                      `json:"skills,omitempty"`
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
			cfg := &GlobalConfig{Agents: make(map[string]GlobalAgentConfig)}
			EnsureDefaultRoleCatalog(cfg)
			EnsureDefaultSkillCatalog(cfg)
			return cfg, nil
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
	EnsureDefaultRoleCatalog(&cfg)
	EnsureDefaultSkillCatalog(&cfg)
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
	EnsureDefaultRoleCatalog(cfg)
	EnsureDefaultSkillCatalog(cfg)

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

// AddLoop appends a loop definition. Returns an error if the name already exists.
func (c *GlobalConfig) AddLoop(l LoopDef) error {
	for _, existing := range c.Loops {
		if strings.EqualFold(existing.Name, l.Name) {
			return errors.New("loop already exists: " + l.Name)
		}
	}
	c.Loops = append(c.Loops, l)
	return nil
}

// RemoveLoop removes a loop by name (case-insensitive).
func (c *GlobalConfig) RemoveLoop(name string) {
	out := c.Loops[:0]
	for _, l := range c.Loops {
		if !strings.EqualFold(l.Name, name) {
			out = append(out, l)
		}
	}
	c.Loops = out
}

// FindLoop returns a pointer to a loop definition by name, or nil if not found.
func (c *GlobalConfig) FindLoop(name string) *LoopDef {
	for i := range c.Loops {
		if strings.EqualFold(c.Loops[i].Name, name) {
			return &c.Loops[i]
		}
	}
	return nil
}

// AddTeam appends a team. Returns an error if the name already exists.
func (c *GlobalConfig) AddTeam(t Team) error {
	for _, existing := range c.Teams {
		if strings.EqualFold(existing.Name, t.Name) {
			return errors.New("team already exists: " + t.Name)
		}
	}
	c.Teams = append(c.Teams, t)
	return nil
}

// RemoveTeam removes a team by name (case-insensitive).
func (c *GlobalConfig) RemoveTeam(name string) {
	out := c.Teams[:0]
	for _, t := range c.Teams {
		if !strings.EqualFold(t.Name, name) {
			out = append(out, t)
		}
	}
	c.Teams = out
}

// FindTeam returns a pointer to a team by name, or nil if not found.
func (c *GlobalConfig) FindTeam(name string) *Team {
	for i := range c.Teams {
		if strings.EqualFold(c.Teams[i].Name, name) {
			return &c.Teams[i]
		}
	}
	return nil
}

const maxRecentCombinations = 20

// RecordRecentCombination adds or bumps a profile+team combination to the top of recent list.
func (c *GlobalConfig) RecordRecentCombination(profile, team string) {
	now := time.Now().UTC()

	// Remove existing entry for this combination.
	out := make([]RecentCombination, 0, len(c.RecentCombinations))
	for _, rc := range c.RecentCombinations {
		if !(strings.EqualFold(rc.Profile, profile) && strings.EqualFold(rc.Team, team)) {
			out = append(out, rc)
		}
	}

	// Prepend new entry.
	out = append([]RecentCombination{{Profile: profile, Team: team, UsedAt: now}}, out...)

	// Cap at max.
	if len(out) > maxRecentCombinations {
		out = out[:maxRecentCombinations]
	}

	// Sort by most recent first.
	sort.Slice(out, func(i, j int) bool { return out[i].UsedAt.After(out[j].UsedAt) })

	c.RecentCombinations = out
}

const maxRecentProjects = 50

// RecordRecentProject adds or bumps a project to the top of the recent projects list.
func (c *GlobalConfig) RecordRecentProject(id, path, name, rootDir string) {
	now := time.Now().UTC()

	// Remove existing entry for this path.
	out := make([]RecentProject, 0, len(c.RecentProjects))
	for _, rp := range c.RecentProjects {
		if rp.Path != path {
			out = append(out, rp)
		}
	}

	// Prepend new entry.
	out = append([]RecentProject{{ID: id, Path: path, Name: name, RootDir: rootDir, OpenedAt: now}}, out...)

	// Cap at max.
	if len(out) > maxRecentProjects {
		out = out[:maxRecentProjects]
	}

	// Sort by most recent first.
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.After(out[j].OpenedAt) })

	c.RecentProjects = out
}

// RemoveRecentProject removes a recent project by path.
func (c *GlobalConfig) RemoveRecentProject(path string) {
	out := c.RecentProjects[:0]
	for _, rp := range c.RecentProjects {
		if rp.Path != path {
			out = append(out, rp)
		}
	}
	c.RecentProjects = out
}

// FindSkill returns a pointer to a skill by ID, or nil if not found.
func (c *GlobalConfig) FindSkill(id string) *Skill {
	if c == nil {
		return nil
	}
	EnsureDefaultSkillCatalog(c)
	key := normalizeSkillID(id)
	for i := range c.Skills {
		if normalizeSkillID(c.Skills[i].ID) == key {
			return &c.Skills[i]
		}
	}
	return nil
}

// AddSkill appends a skill. Returns an error if the ID already exists.
func (c *GlobalConfig) AddSkill(sk Skill) error {
	if c == nil {
		return errors.New("global config is nil")
	}
	EnsureDefaultSkillCatalog(c)
	id := normalizeSkillID(sk.ID)
	if id == "" {
		return errors.New("skill id cannot be empty")
	}
	if c.FindSkill(id) != nil {
		return errors.New("skill already exists: " + id)
	}
	sk.ID = id
	c.Skills = append(c.Skills, sk)
	return nil
}

// RemoveSkill removes a skill by ID (case-insensitive).
func (c *GlobalConfig) RemoveSkill(id string) {
	if c == nil {
		return
	}
	EnsureDefaultSkillCatalog(c)
	key := normalizeSkillID(id)
	out := c.Skills[:0]
	for _, sk := range c.Skills {
		if normalizeSkillID(sk.ID) != key {
			out = append(out, sk)
		}
	}
	c.Skills = out
}
