package config

import "strings"

const (
	ProfileCostFree      = "free"
	ProfileCostCheap     = "cheap"
	ProfileCostNormal    = "normal"
	ProfileCostExpensive = "expensive"
)

var allowedProfileCosts = map[string]struct{}{
	ProfileCostFree:      {},
	ProfileCostCheap:     {},
	ProfileCostNormal:    {},
	ProfileCostExpensive: {},
}

// NormalizeProfileCost trims and lowercases a profile cost tier.
func NormalizeProfileCost(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

// ValidProfileCost reports whether v is one of the supported profile cost tiers.
func ValidProfileCost(v string) bool {
	_, ok := allowedProfileCosts[NormalizeProfileCost(v)]
	return ok
}

// AllowedProfileCosts returns all supported profile cost tiers in display order.
func AllowedProfileCosts() []string {
	return []string{
		ProfileCostFree,
		ProfileCostCheap,
		ProfileCostNormal,
		ProfileCostExpensive,
	}
}
