package config

import (
	"fmt"
	"strings"
)

const (
	ResourcePriorityQuality = "quality"
	ResourcePriorityNormal  = "normal"
	ResourcePriorityCost    = "cost"
)

var allowedResourcePriorities = map[string]struct{}{
	ResourcePriorityQuality: {},
	ResourcePriorityNormal:  {},
	ResourcePriorityCost:    {},
}

// NormalizeResourcePriority trims and lowercases a resource-priority value.
func NormalizeResourcePriority(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

// ValidResourcePriority reports whether v is one of quality, normal, or cost.
func ValidResourcePriority(v string) bool {
	_, ok := allowedResourcePriorities[NormalizeResourcePriority(v)]
	return ok
}

// AllowedResourcePriorities returns supported values in display order.
func AllowedResourcePriorities() []string {
	return []string{
		ResourcePriorityQuality,
		ResourcePriorityNormal,
		ResourcePriorityCost,
	}
}

// EffectiveResourcePriority returns a normalized priority, defaulting to normal.
func EffectiveResourcePriority(v string) string {
	normalized := NormalizeResourcePriority(v)
	if normalized == "" {
		return ResourcePriorityNormal
	}
	if !ValidResourcePriority(normalized) {
		return ResourcePriorityNormal
	}
	return normalized
}

// ParseResourcePriority validates and normalizes a priority value.
// Empty input resolves to the default "normal".
func ParseResourcePriority(v string) (string, error) {
	normalized := NormalizeResourcePriority(v)
	if normalized == "" {
		return ResourcePriorityNormal, nil
	}
	if !ValidResourcePriority(normalized) {
		return "", fmt.Errorf("invalid priority %q (allowed: quality, normal, cost)", strings.TrimSpace(v))
	}
	return normalized, nil
}
