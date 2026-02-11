package agent

import "sync"

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
		"claude":  NewClaudeAgent(),
		"codex":   NewCodexAgent(),
		"vibe":    NewVibeAgent(),
		"generic": NewGenericAgent("generic"),
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

// Register adds or replaces an agent in the global registry.
func Register(name string, a Agent) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = a
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
