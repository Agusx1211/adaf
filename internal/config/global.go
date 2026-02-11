package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// GlobalConfig holds user-level preferences stored in ~/.adaf/config.json.
type GlobalConfig struct {
	Agents map[string]GlobalAgentConfig `json:"agents,omitempty"`
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
