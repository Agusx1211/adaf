package agent

import (
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   map[string]Agent
)

func init() {
	registry = DefaultRegistry()
}

// DefaultRegistry returns a map of all built-in agent implementations
// keyed by their canonical names.
func DefaultRegistry() map[string]Agent {
	return map[string]Agent{
		"claude":   NewClaudeAgent(),
		"codex":    NewCodexAgent(),
		"vibe":     NewVibeAgent(),
		"opencode": NewOpencodeAgent(),
		"gemini":   NewGeminiAgent(),
		"generic":  NewGenericAgent("generic"),
	}
}

// Get looks up an agent by name from the global registry.
// Returns the agent and true if found, or nil and false otherwise.
func Get(name string) (Agent, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := registry[name]
	return a, ok
}

// All returns a copy of the current registry.
func All() map[string]Agent {
	registryMu.RLock()
	defer registryMu.RUnlock()
	cp := make(map[string]Agent, len(registry))
	for k, v := range registry {
		cp[k] = v
	}
	return cp
}

// PopulateFromConfig adds agents found in the persisted config to the registry
// as generic agents (if not already registered). This avoids running PATH
// detection — it only uses the previously cached ~/.adaf/agents.json.
func PopulateFromConfig(cfg *AgentsConfig) {
	if cfg == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for name, rec := range cfg.Agents {
		if name == "" || !rec.Detected {
			continue
		}
		if _, exists := registry[name]; exists {
			continue
		}
		registry[name] = NewGenericAgent(name)
	}
}
