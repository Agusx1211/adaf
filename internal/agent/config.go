package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

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
func SyncDetectedAgents(adafRoot string, detected []detect.DetectedAgent) (*AgentsConfig, error) {
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

	if err := SaveAgentsConfig(adafRoot, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadAndSyncAgentsConfig scans local agents and persists the merged view.
func LoadAndSyncAgentsConfig(adafRoot string) (*AgentsConfig, error) {
	detected, err := detect.Scan()
	if err != nil {
		return nil, err
	}
	return SyncDetectedAgents(adafRoot, detected)
}

// SetModelOverride sets a user's default-model override for an agent.
func SetModelOverride(adafRoot, agentName, model string) (*AgentsConfig, error) {
	cfg, err := LoadAndSyncAgentsConfig(adafRoot)
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
func ResolveDefaultModel(cfg *AgentsConfig, agentName string) string {
	agentName = normalizeAgentName(agentName)
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
	return DefaultModel(agentName)
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
