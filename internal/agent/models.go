package agent

import (
	"strings"

	"github.com/agusx1211/adaf/internal/agentmeta"
)

// SupportedModels returns the known models for an agent.
func SupportedModels(name string) []string {
	info, ok := agentmeta.InfoFor(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return nil
	}
	return append([]string(nil), info.SupportedModels...)
}

// DefaultModel returns the default model for an agent.
func DefaultModel(name string) string {
	info, ok := agentmeta.InfoFor(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		return ""
	}
	return info.DefaultModel
}

// Capabilities returns supported capabilities for an agent.
func Capabilities(name string) []string {
	info, ok := agentmeta.InfoFor(strings.ToLower(strings.TrimSpace(name)))
	if !ok {
		fallback, _ := agentmeta.InfoFor("generic")
		return append([]string(nil), fallback.Capabilities...)
	}
	return append([]string(nil), info.Capabilities...)
}

// IsModelSupported reports whether the model appears in the known model list.
// Agents that support arbitrary provider models (like opencode/generic) accept any model.
func IsModelSupported(agentName, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}

	agentName = strings.ToLower(strings.TrimSpace(agentName))
	if agentName == "opencode" || agentName == "generic" {
		return true
	}

	for _, m := range SupportedModels(agentName) {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}
