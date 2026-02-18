package prompt

import (
	"fmt"
	"sort"
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
func delegationSection(deleg *config.DelegationConfig, globalCfg *config.GlobalConfig, runningSpawns []store.SpawnRecord, resourcePriority string) string {
	if deleg == nil || len(deleg.Profiles) == 0 {
		return ""
	}

	var b strings.Builder
	runningByProfile := make(map[string]int)
	runningByOption := make(map[string]int)
	for _, rec := range runningSpawns {
		profileKey := strings.ToLower(strings.TrimSpace(rec.ChildProfile))
		if profileKey != "" {
			runningByProfile[profileKey]++
		}
		if key := delegationLimitOptionKey(rec.ChildProfile, rec.ChildPosition, rec.ChildRole); key != "" {
			runningByOption[key]++
		}
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

	priority := config.NormalizeResourcePriority(resourcePriority)
	if priority != "" {
		if !config.ValidResourcePriority(priority) {
			priority = config.ResourcePriorityNormal
		}
		b.WriteString("## Resource Allocation Priority\n\n")
		fmt.Fprintf(&b, "Current priority: **%s**.\n\n", priority)
		switch priority {
		case config.ResourcePriorityCost:
			b.WriteString("- Default to `free`/`cheap` profiles for implementation and iteration.\n")
			b.WriteString("- Use `normal`/`expensive` profiles only when cheaper profiles are truly stuck or repeatedly failing quality checks.\n\n")
		case config.ResourcePriorityQuality:
			b.WriteString("- Default to the highest-quality (`expensive`/highest-intelligence) profiles for implementation and key decisions.\n")
			b.WriteString("- Keep `free`/`cheap` profiles for review, QA, and scouting/research passes.\n\n")
		default:
			b.WriteString("- Balance quality and cost by matching task difficulty to profile capability.\n\n")
		}
	}

	// Command reference pointer — full command reference available via `adaf skill delegation`.
	b.WriteString("Run `adaf skill delegation` for command reference and spawn patterns.\n\n")

	// Routing discipline — prevent single-profile spam.
	b.WriteString("## Routing Discipline\n\n")
	b.WriteString("You MUST distribute work across available profiles. Do not repeatedly spawn the same profile.\n\n")
	if priority == config.ResourcePriorityQuality {
		b.WriteString("- **Match difficulty to quality-first policy:** use stronger profiles by default and keep cheaper profiles focused on scouting/review.\n")
	} else if priority == config.ResourcePriorityCost {
		b.WriteString("- **Match difficulty to cost-first policy:** start with cheaper/faster profiles and escalate only after concrete stuck signals.\n")
	} else {
		b.WriteString("- **Match difficulty to cost:** use cheaper/faster profiles for straightforward tasks, reserve expensive/high-intelligence profiles for genuinely hard problems.\n")
	}
	b.WriteString("- **Rotate profiles:** if you just spawned profile X, consider a different profile for the next task unless X is the only option or clearly the best fit.\n")
	b.WriteString("- **Consult the scoreboard:** use the Routing Scoreboard below to pick the best profile for each role, not just the one you used last.\n")
	b.WriteString("- **Avoid concentration:** spreading work across profiles prevents draining a single provider's usage quota and improves overall throughput.\n\n")

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
			optionMaxInstances := dp.MaxInstances
			if optionMaxInstances > 0 {
				line += fmt.Sprintf(", max_instances=%d", optionMaxInstances)
			}
			profileMaxInstances := p.MaxInstances
			if profileMaxInstances > 0 {
				line += fmt.Sprintf(", profile_max_instances=%d", profileMaxInstances)
			}
			runningProfile := runningByProfile[strings.ToLower(strings.TrimSpace(p.Name))]
			if optionMaxInstances > 0 && len(roles) == 1 {
				runningOption := runningByOption[delegationLimitOptionKey(p.Name, config.PositionWorker, roles[0])]
				line += fmt.Sprintf(", running=%d/%d", runningOption, optionMaxInstances)
				if runningOption >= optionMaxInstances {
					line += " [at-cap]"
				}
				if profileMaxInstances > 0 {
					line += fmt.Sprintf(", profile_running=%d/%d", runningProfile, profileMaxInstances)
					if runningProfile >= profileMaxInstances {
						line += " [profile-at-cap]"
					}
				}
			} else if profileMaxInstances > 0 {
				line += fmt.Sprintf(", running=%d/%d", runningProfile, profileMaxInstances)
				if runningProfile >= profileMaxInstances {
					line += " [at-cap]"
				}
			} else if runningProfile > 0 {
				line += fmt.Sprintf(", running=%d", runningProfile)
			}
			if dp.Handoff {
				line += " [handoff]"
			}
			if perf, ok := perfByProfile[strings.ToLower(strings.TrimSpace(p.Name))]; ok && perf.TotalFeedback > 0 {
				line += fmt.Sprintf(", feedback=%d, score=%.1f/100, speed_score=%.0f/100", perf.TotalFeedback, perf.Score, perf.SpeedScore)
				line += fmt.Sprintf(", q=%.2f/10, diff=%.2f/10", perf.AvgQuality, perf.AvgDifficulty)
				if perf.AvgDurationSecs > 0 {
					line += fmt.Sprintf(", avg_dur=%s", formatDelegationDuration(perf.AvgDurationSecs))
				}
				if topRoles := formatDelegationRoleScores(perf.RoleBreakdown, 2); topRoles != "" {
					line += fmt.Sprintf(", role_scores=%s", topRoles)
				}
			}
			if p.Description != "" {
				line += fmt.Sprintf(" — %s", p.Description)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	profileRows, roleColumns, roleMatrixRows := buildDelegationRoutingTables(deleg, globalCfg, perfByProfile)
	if len(profileRows) > 0 && len(roleColumns) > 0 {
		b.WriteString("## Routing Scoreboard (difficulty-adjusted, judge-calibrated, judge-weighted)\n\n")
		b.WriteString("### Profile Baseline\n\n")
		b.WriteString("| Profile | Cost | Speed |\n")
		b.WriteString("| --- | --- | ---: |\n")
		for _, row := range profileRows {
			fmt.Fprintf(&b, "| %s | %s | ", row.Profile, row.Cost)
			if row.HasSpeedScore {
				fmt.Fprintf(&b, "%.0f/100 |\n", row.SpeedScore)
			} else {
				b.WriteString("... |\n")
			}
		}
		b.WriteString("\n")

		b.WriteString("### Score by Role\n\n")
		b.WriteString("| Profile |")
		for _, role := range roleColumns {
			fmt.Fprintf(&b, " %s |", role)
		}
		b.WriteString("\n")
		b.WriteString("| --- |")
		for range roleColumns {
			b.WriteString(" ---: |")
		}
		b.WriteString("\n")

		matrixByProfile := make(map[string]delegationRoutingRoleMatrixRow, len(roleMatrixRows))
		for _, row := range roleMatrixRows {
			matrixByProfile[strings.ToLower(strings.TrimSpace(row.Profile))] = row
		}
		for _, row := range profileRows {
			fmt.Fprintf(&b, "| %s |", row.Profile)
			matrixRow := matrixByProfile[strings.ToLower(strings.TrimSpace(row.Profile))]
			for _, role := range roleColumns {
				if _, available := matrixRow.AvailableRoles[role]; !available {
					b.WriteString(" -- |")
					continue
				}
				if score, ok := matrixRow.Scores[role]; ok {
					fmt.Fprintf(&b, " %.1f/100 |", score)
				} else {
					b.WriteString(" ... |")
				}
			}
			b.WriteString("\n")
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

func formatDelegationRoleScores(items []profilescore.BreakdownStats, limit int) string {
	if len(items) == 0 || limit <= 0 {
		return ""
	}
	sorted := append([]profilescore.BreakdownStats(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return strings.ToLower(sorted[i].Name) < strings.ToLower(sorted[j].Name)
	})
	if limit > len(items) {
		limit = len(sorted)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%s:%.1f(%d)", sorted[i].Name, sorted[i].Score, sorted[i].Count))
	}
	return strings.Join(parts, "|")
}

func delegationLimitOptionKey(profile, position, role string) string {
	prof := strings.ToLower(strings.TrimSpace(profile))
	pos := strings.ToLower(strings.TrimSpace(position))
	r := strings.ToLower(strings.TrimSpace(role))
	if prof == "" {
		return ""
	}
	if pos == "" {
		pos = config.PositionWorker
	}
	if r == "" {
		r = "-"
	}
	return prof + "|" + pos + "|" + r
}

type delegationRoutingProfileRow struct {
	Profile       string
	Cost          string
	SpeedScore    float64
	HasSpeedScore bool
}

type delegationRoutingRoleMatrixRow struct {
	Profile        string
	AvailableRoles map[string]struct{}
	Scores         map[string]float64
}

func buildDelegationRoutingTables(deleg *config.DelegationConfig, globalCfg *config.GlobalConfig, perfByProfile map[string]profilescore.ProfileSummary) ([]delegationRoutingProfileRow, []string, []delegationRoutingRoleMatrixRow) {
	if deleg == nil || globalCfg == nil || len(deleg.Profiles) == 0 {
		return nil, nil, nil
	}
	profileRows := make([]delegationRoutingProfileRow, 0)
	roleMatrixRows := make([]delegationRoutingRoleMatrixRow, 0)
	roleColumnsSet := make(map[string]struct{})
	profileRowIndex := make(map[string]int)
	for _, dp := range deleg.Profiles {
		p := globalCfg.FindProfile(dp.Name)
		if p == nil {
			continue
		}
		profileKey := strings.ToLower(strings.TrimSpace(p.Name))
		if profileKey == "" {
			continue
		}
		rowIndex, exists := profileRowIndex[profileKey]
		if !exists {
			cost := config.NormalizeProfileCost(p.Cost)
			if cost == "" {
				cost = "n/a"
			}
			row := delegationRoutingProfileRow{
				Profile: p.Name,
				Cost:    cost,
			}
			if perf, ok := perfByProfile[profileKey]; ok && perf.TotalFeedback > 0 {
				row.SpeedScore = perf.SpeedScore
				row.HasSpeedScore = true
			}
			profileRows = append(profileRows, row)
			roleMatrixRows = append(roleMatrixRows, delegationRoutingRoleMatrixRow{
				Profile:        p.Name,
				AvailableRoles: make(map[string]struct{}),
				Scores:         make(map[string]float64),
			})
			rowIndex = len(profileRows) - 1
			profileRowIndex[profileKey] = rowIndex
		}
		roles, err := dp.EffectiveRoles()
		if err != nil {
			continue
		}
		for _, role := range roles {
			roleName := strings.ToLower(strings.TrimSpace(role))
			if roleName == "" {
				continue
			}
			roleColumnsSet[roleName] = struct{}{}
			roleMatrixRows[rowIndex].AvailableRoles[roleName] = struct{}{}
		}
	}

	for i, row := range profileRows {
		profileKey := strings.ToLower(strings.TrimSpace(row.Profile))
		perf, ok := perfByProfile[profileKey]
		if !ok {
			continue
		}
		for _, role := range perf.RoleBreakdown {
			roleName := strings.ToLower(strings.TrimSpace(role.Name))
			if role.Count <= 0 || roleName == "" {
				continue
			}
			if _, available := roleMatrixRows[i].AvailableRoles[roleName]; !available {
				continue
			}
			roleMatrixRows[i].Scores[roleName] = role.Score
		}
	}

	if len(profileRows) == 0 || len(roleColumnsSet) == 0 {
		return nil, nil, nil
	}

	roleColumns := make([]string, 0, len(roleColumnsSet))
	for role := range roleColumnsSet {
		roleColumns = append(roleColumns, role)
	}
	sort.Slice(roleColumns, func(i, j int) bool {
		return roleColumns[i] < roleColumns[j]
	})
	return profileRows, roleColumns, roleMatrixRows
}
