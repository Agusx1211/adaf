package prompt

import (
	"fmt"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/profilescore"
	"github.com/agusx1211/adaf/internal/store"
)

// RolePrompt returns the role-specific system prompt section for a profile.
// An agent NEVER sees its own intelligence rating.
// Spawning capabilities are no longer emitted here — they come from delegationSection().
func RolePrompt(profile *config.Profile, stepRole string, globalCfg *config.GlobalConfig) string {
	role := config.EffectiveStepRole(stepRole, globalCfg)

	roles := config.DefaultRoleDefinitions()
	rules := config.DefaultPromptRules()
	if globalCfg != nil {
		config.EnsureDefaultRoleCatalog(globalCfg)
		roles = globalCfg.Roles
		rules = globalCfg.PromptRules
	}

	ruleBodies := make(map[string]string, len(rules))
	for _, rule := range rules {
		ruleID := strings.ToLower(strings.TrimSpace(rule.ID))
		if ruleID == "" {
			continue
		}
		ruleBodies[ruleID] = strings.TrimSpace(rule.Body)
	}

	roleTitle := strings.ToUpper(role)
	roleIdentity := ""
	roleDesc := ""
	var ruleIDs []string
	for _, def := range roles {
		if strings.EqualFold(def.Name, role) {
			if strings.TrimSpace(def.Title) != "" {
				roleTitle = strings.TrimSpace(def.Title)
			}
			roleIdentity = strings.TrimSpace(def.Identity)
			roleDesc = strings.TrimSpace(def.Description)
			ruleIDs = append([]string(nil), def.RuleIDs...)
			break
		}
	}

	var b strings.Builder
	b.WriteString("# Your Role: " + roleTitle + "\n\n")
	if roleIdentity != "" {
		b.WriteString(roleIdentity + "\n\n")
	}
	if roleDesc != "" {
		b.WriteString(roleDesc + "\n\n")
	}
	for _, ruleID := range ruleIDs {
		// Downstream communication guidance is injected by delegationSection()
		// when delegation is actually available.
		if strings.EqualFold(strings.TrimSpace(ruleID), config.RuleCommunicationDownstream) {
			continue
		}
		body := ruleBodies[strings.ToLower(strings.TrimSpace(ruleID))]
		if body == "" {
			continue
		}
		b.WriteString(body + "\n\n")
	}

	return b.String()
}

// RolePromptSlim returns a simplified role section with only title, identity, and description.
// Used by the skills-driven prompt path where rule composition is replaced by skill blocks.
func RolePromptSlim(profile *config.Profile, stepRole string, globalCfg *config.GlobalConfig) string {
	role := config.EffectiveStepRole(stepRole, globalCfg)

	roles := config.DefaultRoleDefinitions()
	if globalCfg != nil {
		config.EnsureDefaultRoleCatalog(globalCfg)
		roles = globalCfg.Roles
	}

	roleTitle := strings.ToUpper(role)
	roleIdentity := ""
	roleDesc := ""
	for _, def := range roles {
		if strings.EqualFold(def.Name, role) {
			if strings.TrimSpace(def.Title) != "" {
				roleTitle = strings.TrimSpace(def.Title)
			}
			roleIdentity = strings.TrimSpace(def.Identity)
			roleDesc = strings.TrimSpace(def.Description)
			break
		}
	}

	var b strings.Builder
	b.WriteString("# Your Role: " + roleTitle + "\n\n")
	if roleIdentity != "" {
		b.WriteString(roleIdentity + "\n\n")
	}
	if roleDesc != "" {
		b.WriteString(roleDesc + "\n\n")
	}

	return b.String()
}

// ReadOnlyPrompt returns the read-only mode prompt section.
func ReadOnlyPrompt() string {
	return "# READ-ONLY MODE\n\nYou are in READ-ONLY mode. Do NOT create, modify, or delete any files. Only read and analyze.\n\nDo NOT write reports into repository files (for example `*.md`, `*.txt`, or TODO files). Return your report in your final assistant message.\n"
}

// delegationSection builds the delegation/spawning prompt section from a DelegationConfig.
func delegationSection(deleg *config.DelegationConfig, globalCfg *config.GlobalConfig, runningSpawns []store.SpawnRecord) string {
	if deleg == nil || len(deleg.Profiles) == 0 {
		return ""
	}

	var b strings.Builder
	runningByProfile := make(map[string]int)
	for _, rec := range runningSpawns {
		runningByProfile[strings.ToLower(strings.TrimSpace(rec.ChildProfile))]++
	}

	b.WriteString("# Delegation\n\n")

	// Downstream communication style — only shown when delegation is available.
	if globalCfg != nil {
		if rule := globalCfg.FindPromptRule(config.RuleCommunicationDownstream); rule != nil && rule.Body != "" {
			b.WriteString(rule.Body + "\n\n")
		}
	}

	// Style guidance.
	if style := deleg.DelegationStyleText(); style != "" {
		b.WriteString("**Delegation style:** " + style + "\n\n")
	}

	// Command reference pointer — full command reference available via `adaf skill delegation`.
	b.WriteString("Run `adaf skill delegation` for command reference and spawn patterns.\n\n")
	perfByProfile := loadDelegationPerformance(globalCfg)

	if len(runningSpawns) > 0 {
		b.WriteString("## Currently Running Spawns\n\n")
		for _, rec := range runningSpawns {
			line := fmt.Sprintf("- Spawn #%d — profile=%s", rec.ID, rec.ChildProfile)
			if strings.TrimSpace(rec.ChildRole) != "" {
				line += fmt.Sprintf(", role=%s", rec.ChildRole)
			}
			line += fmt.Sprintf(", status=%s", rec.Status)
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	// Available profiles.
	if len(deleg.Profiles) > 0 && globalCfg != nil {
		b.WriteString("## Available Profiles to Spawn\n\n")
		for _, dp := range deleg.Profiles {
			p := globalCfg.FindProfile(dp.Name)
			if p == nil {
				fmt.Fprintf(&b, "- **%s** (not found in config)\n", dp.Name)
				continue
			}
			line := fmt.Sprintf("- **%s** — agent=%s", p.Name, p.Agent)
			roles, rolesErr := dp.EffectiveRoles()
			if rolesErr != nil {
				line += fmt.Sprintf(", roles=INVALID(%v)", rolesErr)
			} else if len(roles) == 1 {
				line += fmt.Sprintf(", role=%s", roles[0])
			} else if len(roles) > 1 {
				line += fmt.Sprintf(", roles=%s", strings.Join(roles, "/"))
			}
			if p.Model != "" {
				line += fmt.Sprintf(", model=%s", p.Model)
			}
			if p.Intelligence > 0 {
				line += fmt.Sprintf(", intelligence=%d/10", p.Intelligence)
			}
			if cost := config.NormalizeProfileCost(p.Cost); cost != "" {
				line += fmt.Sprintf(", cost=%s", cost)
			}
			speed := dp.Speed
			if speed == "" {
				speed = p.Speed
			}
			if speed != "" {
				line += fmt.Sprintf(", speed=%s", speed)
			}
			maxInstances := p.MaxInstances
			if dp.MaxInstances > 0 {
				maxInstances = dp.MaxInstances
			}
			if maxInstances > 0 {
				line += fmt.Sprintf(", max_instances=%d", maxInstances)
			}
			running := runningByProfile[strings.ToLower(strings.TrimSpace(p.Name))]
			if maxInstances > 0 {
				line += fmt.Sprintf(", running=%d/%d", running, maxInstances)
				if running >= maxInstances {
					line += " [at-cap]"
				}
			} else if running > 0 {
				line += fmt.Sprintf(", running=%d", running)
			}
			if dp.Handoff {
				line += " [handoff]"
			}
			if perf, ok := perfByProfile[strings.ToLower(strings.TrimSpace(p.Name))]; ok && perf.TotalFeedback > 0 {
				line += fmt.Sprintf(", feedback=%d, q=%.2f/10, diff=%.2f/10", perf.TotalFeedback, perf.AvgQuality, perf.AvgDifficulty)
				if perf.AvgDurationSecs > 0 {
					line += fmt.Sprintf(", avg_dur=%s", formatDelegationDuration(perf.AvgDurationSecs))
				}
				if topRoles := formatDelegationRoleQuality(perf.RoleBreakdown, 2); topRoles != "" {
					line += fmt.Sprintf(", role_quality=%s", topRoles)
				}
			}
			if p.Description != "" {
				line += fmt.Sprintf(" — %s", p.Description)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	maxPar := deleg.EffectiveMaxParallel()
	fmt.Fprintf(&b, "Maximum concurrent sub-agents: %d\n\n", maxPar)

	return b.String()
}

func loadDelegationPerformance(globalCfg *config.GlobalConfig) map[string]profilescore.ProfileSummary {
	out := make(map[string]profilescore.ProfileSummary)
	if globalCfg == nil {
		return out
	}
	records, err := profilescore.Default().ListFeedback()
	if err != nil {
		return out
	}
	catalog := make([]profilescore.ProfileCatalogEntry, 0, len(globalCfg.Profiles))
	for _, prof := range globalCfg.Profiles {
		catalog = append(catalog, profilescore.ProfileCatalogEntry{
			Name: prof.Name,
			Cost: config.NormalizeProfileCost(prof.Cost),
		})
	}
	report := profilescore.BuildDashboard(catalog, records)
	for _, s := range report.Profiles {
		out[strings.ToLower(strings.TrimSpace(s.Profile))] = s
	}
	return out
}

func formatDelegationDuration(avgDurationSecs float64) string {
	if avgDurationSecs <= 0 {
		return "n/a"
	}
	d := time.Duration(avgDurationSecs * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", avgDurationSecs)
	}
	return d.Round(time.Second).String()
}

func formatDelegationRoleQuality(items []profilescore.BreakdownStats, limit int) string {
	if len(items) == 0 || limit <= 0 {
		return ""
	}
	if limit > len(items) {
		limit = len(items)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%s:%.2f(%d)", items[i].Name, items[i].AvgQuality, items[i].Count))
	}
	return strings.Join(parts, "|")
}
