package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/detect"
)

// AgentsConfig persists detected agent state and user overrides.
type AgentsConfig struct {
	Updated time.Time              `json:"updated"`
	Agents  map[string]AgentRecord `json:"agents"`
}

// AgentRecord holds merged detection metadata and user model overrides.
type AgentRecord struct {
	Name            string    `json:"name"`
	Path            string    `json:"path,omitempty"`
	Version         string    `json:"version,omitempty"`
	Capabilities    []string  `json:"capabilities,omitempty"`
	SupportedModels []string  `json:"supported_models,omitempty"`
	DefaultModel    string    `json:"default_model,omitempty"`
	ModelOverride   string    `json:"model_override,omitempty"`
	Detected        bool      `json:"detected"`
	DetectedAt      time.Time `json:"detected_at,omitempty"`
}

// AgentsConfigPath returns the full path to .adaf/agents.json.
func AgentsConfigPath(adafRoot string) string {
	if strings.TrimSpace(adafRoot) == "" {
		adafRoot = ".adaf"
	}
	return filepath.Join(adafRoot, "agents.json")
}

// LoadAgentsConfig loads .adaf/agents.json, returning an empty config if absent.
func LoadAgentsConfig(adafRoot string) (*AgentsConfig, error) {
	path := AgentsConfigPath(adafRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AgentsConfig{Agents: make(map[string]AgentRecord)}, nil
		}
		return nil, err
	}

	var cfg AgentsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentRecord)
	}
	return &cfg, nil
}

// SaveAgentsConfig writes .adaf/agents.json.
func SaveAgentsConfig(adafRoot string, cfg *AgentsConfig) error {
	if cfg == nil {
		cfg = &AgentsConfig{}
	}
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]AgentRecord)
	}
	cfg.Updated = time.Now().UTC()

	if err := os.MkdirAll(filepath.Dir(AgentsConfigPath(adafRoot)), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(AgentsConfigPath(adafRoot), data, 0644)
}

// SyncDetectedAgents merges fresh detection results into persisted agent config.
// If globalCfg is non-nil, agent paths from the global config are used as fallback
// when detection doesn't find a path.
func SyncDetectedAgents(adafRoot string, detected []detect.DetectedAgent, globalCfg *config.GlobalConfig) (*AgentsConfig, error) {
	cfg, err := LoadAgentsConfig(adafRoot)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	seen := make(map[string]struct{}, len(detected))

	for _, d := range detected {
		name := normalizeAgentName(d.Name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}

		rec := cfg.Agents[name]
		rec.Name = name
		rec.Path = d.Path
		rec.Version = d.Version
		rec.Capabilities = append([]string(nil), d.Capabilities...)
		rec.SupportedModels = append([]string(nil), d.SupportedModels...)
		rec.Detected = true
		rec.DetectedAt = now
		rec.DefaultModel = effectiveDefaultModel(name, rec.ModelOverride)
		cfg.Agents[name] = rec
	}

	for name, rec := range cfg.Agents {
		if _, ok := seen[name]; ok {
			continue
		}
		rec.Detected = false
		if len(rec.Capabilities) == 0 {
			rec.Capabilities = Capabilities(name)
		}
		if len(rec.SupportedModels) == 0 {
			rec.SupportedModels = SupportedModels(name)
		}
		rec.DefaultModel = effectiveDefaultModel(name, rec.ModelOverride)
		cfg.Agents[name] = rec
	}

	// Apply global path fallback for agents without a detected path.
	if globalCfg != nil {
		for name, ga := range globalCfg.Agents {
			name = normalizeAgentName(name)
			if strings.TrimSpace(ga.Path) == "" {
				continue
			}
			rec := cfg.Agents[name]
			if strings.TrimSpace(rec.Path) == "" {
				rec.Name = name
				rec.Path = ga.Path
				cfg.Agents[name] = rec
			}
		}
	}

	if err := SaveAgentsConfig(adafRoot, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadAndSyncAgentsConfig scans local agents and persists the merged view.
// If globalCfg is non-nil, global settings are merged in.
func LoadAndSyncAgentsConfig(adafRoot string, globalCfg *config.GlobalConfig) (*AgentsConfig, error) {
	detected, err := detect.Scan()
	if err != nil {
		return nil, err
	}
	return SyncDetectedAgents(adafRoot, detected, globalCfg)
}

// SetModelOverride sets a user's default-model override for an agent.
func SetModelOverride(adafRoot, agentName, model string, globalCfg *config.GlobalConfig) (*AgentsConfig, error) {
	cfg, err := LoadAgentsConfig(adafRoot)
	if err != nil {
		return nil, err
	}

	agentName = normalizeAgentName(agentName)
	model = strings.TrimSpace(model)
	rec := cfg.Agents[agentName]
	rec.Name = agentName
	rec.ModelOverride = model
	rec.DefaultModel = effectiveDefaultModel(agentName, model)
	if len(rec.Capabilities) == 0 {
		rec.Capabilities = Capabilities(agentName)
	}
	if len(rec.SupportedModels) == 0 {
		rec.SupportedModels = SupportedModels(agentName)
	}
	cfg.Agents[agentName] = rec

	if err := SaveAgentsConfig(adafRoot, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ResolveDefaultModel returns the configured model override, falling back to
// built-in defaults for the agent.
// Priority (lowest to highest): built-in defaults, global config, project config.
func ResolveDefaultModel(cfg *AgentsConfig, globalCfg *config.GlobalConfig, agentName string) string {
	agentName = normalizeAgentName(agentName)

	// Project-level override (highest priority of stored config).
	if cfg != nil {
		if rec, ok := cfg.Agents[agentName]; ok {
			if strings.TrimSpace(rec.ModelOverride) != "" {
				return strings.TrimSpace(rec.ModelOverride)
			}
			if strings.TrimSpace(rec.DefaultModel) != "" {
				return strings.TrimSpace(rec.DefaultModel)
			}
		}
	}

	// Global-level override.
	if globalCfg != nil {
		if ga, ok := globalCfg.Agents[agentName]; ok {
			if strings.TrimSpace(ga.ModelOverride) != "" {
				return strings.TrimSpace(ga.ModelOverride)
			}
		}
	}

	return DefaultModel(agentName)
}

// ResolveModelOverride returns only an explicit model override for an agent.
// It checks project config first, then global config. Does not fall back to defaults.
func ResolveModelOverride(cfg *AgentsConfig, globalCfg *config.GlobalConfig, agentName string) string {
	agentName = normalizeAgentName(agentName)

	// Project-level override takes priority.
	if cfg != nil {
		if rec, ok := cfg.Agents[agentName]; ok {
			if strings.TrimSpace(rec.ModelOverride) != "" {
				return strings.TrimSpace(rec.ModelOverride)
			}
		}
	}

	// Global-level override.
	if globalCfg != nil {
		if ga, ok := globalCfg.Agents[agentName]; ok {
			if strings.TrimSpace(ga.ModelOverride) != "" {
				return strings.TrimSpace(ga.ModelOverride)
			}
		}
	}

	return ""
}

func effectiveDefaultModel(agentName, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return DefaultModel(agentName)
}

func normalizeAgentName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
