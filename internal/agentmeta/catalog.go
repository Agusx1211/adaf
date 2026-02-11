package agentmeta

import (
	"sort"
	"strings"
)

// Info describes built-in metadata for an agent tool.
type Info struct {
	Name            string
	Binary          string
	DefaultModel    string
	SupportedModels []string
	Capabilities    []string
}

var builtin = map[string]Info{
	"claude": {
		Name:            "claude",
		Binary:          "claude",
		DefaultModel:    "sonnet-4.5",
		SupportedModels: []string{"opus-4", "sonnet-4.5", "haiku-4.5"},
		Capabilities:    []string{"prompt-arg", "model-select", "auto-approve", "stream-output"},
	},
	"codex": {
		Name:            "codex",
		Binary:          "codex",
		DefaultModel:    "o4-mini",
		SupportedModels: []string{"o3", "o4-mini", "gpt-4.1"},
		Capabilities:    []string{"prompt-arg", "model-select", "auto-approve", "tty-required"},
	},
	"vibe": {
		Name:            "vibe",
		Binary:          "vibe",
		DefaultModel:    "default",
		SupportedModels: []string{"default"},
		Capabilities:    []string{"stdin-prompt", "stream-output"},
	},
	"opencode": {
		Name:            "opencode",
		Binary:          "opencode",
		DefaultModel:    "openai/gpt-4.1",
		SupportedModels: []string{"openai/gpt-4.1", "anthropic/claude-sonnet-4.5", "google/gemini-2.5-pro"},
		Capabilities:    []string{"stdin-prompt", "model-select", "multi-provider", "stream-output"},
	},
	"generic": {
		Name:         "generic",
		Binary:       "",
		DefaultModel: "",
		Capabilities: []string{"stdin-prompt", "stream-output"},
	},
}

// InfoFor returns metadata for an agent name.
func InfoFor(name string) (Info, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	info, ok := builtin[name]
	if !ok {
		return Info{}, false
	}
	return clone(info), true
}

// Names returns known agent names in stable order.
func Names() []string {
	names := make([]string, 0, len(builtin))
	for name := range builtin {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BinaryNames returns the known concrete agent binaries (excluding generic).
func BinaryNames() map[string]string {
	out := make(map[string]string, len(builtin)-1)
	for name, info := range builtin {
		if info.Binary == "" {
			continue
		}
		out[name] = info.Binary
	}
	return out
}

func clone(info Info) Info {
	cp := info
	cp.SupportedModels = append([]string(nil), info.SupportedModels...)
	cp.Capabilities = append([]string(nil), info.Capabilities...)
	return cp
}
