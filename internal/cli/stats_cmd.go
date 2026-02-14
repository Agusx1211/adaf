package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

var statsFormat string

func init() {
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show profile and loop statistics",
		RunE:  runStatsOverview,
	}

	statsProfileCmd := &cobra.Command{
		Use:   "profile [name]",
		Short: "Show detailed stats for a profile",
		Args:  cobra.ExactArgs(1),
		RunE:  runStatsProfile,
	}

	statsLoopCmd := &cobra.Command{
		Use:   "loop [name]",
		Short: "Show detailed stats for a loop",
		Args:  cobra.ExactArgs(1),
		RunE:  runStatsLoop,
	}

	statsMigrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Retroactively compute stats from existing recordings",
		RunE:  runStatsMigrate,
	}

	statsCmd.PersistentFlags().StringVar(&statsFormat, "format", "table",
		"output format: table or markdown")

	statsCmd.AddCommand(statsProfileCmd, statsLoopCmd, statsMigrateCmd)
	rootCmd.AddCommand(statsCmd)
}

func runStatsOverview(cmd *cobra.Command, args []string) error {
	if isMarkdownFormat(statsFormat) {
		return fmt.Errorf("markdown output is only supported for 'stats profile' and 'stats loop'")
	}
	if !isTableFormat(statsFormat) {
		return fmt.Errorf("unsupported stats format %q (use table or markdown)", statsFormat)
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	profileStats, _ := s.ListProfileStats()
	loopStats, _ := s.ListLoopStats()

	if len(profileStats) == 0 && len(loopStats) == 0 {
		fmt.Println(colorDim + "No stats found. Run 'adaf stats migrate' to compute from existing recordings." + colorReset)
		return nil
	}

	if len(profileStats) > 0 {
		printHeader("Profiles")
		headers := []string{"Profile", "Runs", "Cost", "Last Run"}
		var rows [][]string
		for _, ps := range profileStats {
			lastRun := colorDim + "never" + colorReset
			if !ps.LastRunAt.IsZero() {
				lastRun = formatTimeAgo(ps.LastRunAt)
			}
			rows = append(rows, []string{
				styleBoldWhite + ps.ProfileName + colorReset,
				fmt.Sprintf("%d", ps.TotalRuns),
				fmt.Sprintf("$%.2f", ps.TotalCostUSD),
				lastRun,
			})
		}
		printTable(headers, rows)
	}

	if len(loopStats) > 0 {
		printHeader("Loops")
		headers := []string{"Loop", "Cycles", "Cost", "Last Run"}
		var rows [][]string
		for _, ls := range loopStats {
			lastRun := colorDim + "never" + colorReset
			if !ls.LastRunAt.IsZero() {
				lastRun = formatTimeAgo(ls.LastRunAt)
			}
			rows = append(rows, []string{
				styleBoldWhite + ls.LoopName + colorReset,
				fmt.Sprintf("%d", ls.TotalCycles),
				fmt.Sprintf("$%.2f", ls.TotalCostUSD),
				lastRun,
			})
		}
		printTable(headers, rows)
	}

	return nil
}

func runStatsProfile(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	name := args[0]
	if isMarkdownFormat(statsFormat) {
		return runStatsProfileMarkdown(s, name)
	}
	if !isTableFormat(statsFormat) {
		return fmt.Errorf("unsupported stats format %q (use table or markdown)", statsFormat)
	}

	ps, err := s.GetProfileStats(name)
	if err != nil {
		return err
	}

	if ps.TotalRuns == 0 {
		fmt.Printf("%sNo stats found for profile %q. Run 'adaf stats migrate' to compute from existing recordings.%s\n",
			colorDim, name, colorReset)
		return nil
	}

	printHeader(fmt.Sprintf("Profile: %s", name))
	printField("Runs", fmt.Sprintf("%d (%d success, %d failure)", ps.TotalRuns, ps.SuccessCount, ps.FailureCount))
	printField("Total Cost", fmt.Sprintf("$%.2f", ps.TotalCostUSD))
	printField("Total Tokens", fmt.Sprintf("%s input, %s output", formatTokens(ps.TotalInputTok), formatTokens(ps.TotalOutputTok)))
	printField("Total Duration", formatDuration(ps.TotalDuration))

	// Top tools
	if len(ps.ToolCalls) > 0 {
		printField("Top Tools", formatTopTools(ps.ToolCalls))
	}

	// Spawns
	if ps.SpawnsCreated > 0 {
		printField("Sub-agents", fmt.Sprintf("%d created", ps.SpawnsCreated))
	}
	if len(ps.SpawnedBy) > 0 {
		var parts []string
		for parent, count := range ps.SpawnedBy {
			parts = append(parts, fmt.Sprintf("%s (%d)", parent, count))
		}
		printField("Triggered By", strings.Join(parts, ", "))
	}

	// Recent turns
	if len(ps.TurnIDs) > 0 {
		recent := ps.TurnIDs
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		var ids []string
		for _, id := range recent {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		printField("Recent Turns", strings.Join(ids, ", "))
	}

	if !ps.LastRunAt.IsZero() {
		printField("Last Run", formatTimeAgo(ps.LastRunAt))
	}

	return nil
}

func runStatsLoop(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	name := args[0]
	if isMarkdownFormat(statsFormat) {
		return runStatsLoopMarkdown(s, name)
	}
	if !isTableFormat(statsFormat) {
		return fmt.Errorf("unsupported stats format %q (use table or markdown)", statsFormat)
	}

	ls, err := s.GetLoopStats(name)
	if err != nil {
		return err
	}

	if ls.TotalRuns == 0 {
		fmt.Printf("%sNo stats found for loop %q. Run 'adaf stats migrate' to compute from existing recordings.%s\n",
			colorDim, name, colorReset)
		return nil
	}

	printHeader(fmt.Sprintf("Loop: %s", name))
	printField("Runs", fmt.Sprintf("%d", ls.TotalRuns))
	printField("Total Cycles", fmt.Sprintf("%d", ls.TotalCycles))
	printField("Total Cost", fmt.Sprintf("$%.2f", ls.TotalCostUSD))
	printField("Total Duration", formatDuration(ls.TotalDuration))

	if len(ls.StepStats) > 0 {
		var parts []string
		for profile, count := range ls.StepStats {
			parts = append(parts, fmt.Sprintf("%s (%d)", profile, count))
		}
		printField("Step Profiles", strings.Join(parts, ", "))
	}

	if len(ls.TurnIDs) > 0 {
		recent := ls.TurnIDs
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		var ids []string
		for _, id := range recent {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		printField("Recent Turns", strings.Join(ids, ", "))
	}

	if !ls.LastRunAt.IsZero() {
		printField("Last Run", formatTimeAgo(ls.LastRunAt))
	}

	return nil
}

func isMarkdownFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "md", "markdown":
		return true
	default:
		return false
	}
}

func isTableFormat(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "table", "text":
		return true
	default:
		return false
	}
}

func runStatsProfileMarkdown(s *store.Store, name string) error {
	globalCfg, _ := config.Load()
	if globalCfg == nil {
		return fmt.Errorf("global config not found")
	}

	prof := globalCfg.FindProfile(name)
	if prof == nil {
		return fmt.Errorf("profile %q not found", name)
	}

	ps, err := s.GetProfileStats(name)
	if err != nil {
		return err
	}

	fmt.Printf("# Profile Report: %s\n\n", name)

	fmt.Println("## Configuration")
	fmt.Printf("- Agent: %s\n", prof.Agent)
	if prof.Model != "" {
		fmt.Printf("- Model: %s\n", prof.Model)
	}
	if prof.Intelligence > 0 {
		fmt.Printf("- Intelligence: %d/10\n", prof.Intelligence)
	}
	if prof.Description != "" {
		fmt.Printf("- Description: %s\n", prof.Description)
	}
	if prof.ReasoningLevel != "" {
		fmt.Printf("- Reasoning Level: %s\n", prof.ReasoningLevel)
	}
	if prof.Speed != "" {
		fmt.Printf("- Speed: %s\n", prof.Speed)
	}
	fmt.Println()

	if ps.TotalRuns > 0 {
		fmt.Println("## Aggregate Statistics")
		fmt.Printf("- Total Runs: %d (%d success, %d failure)\n", ps.TotalRuns, ps.SuccessCount, ps.FailureCount)
		fmt.Printf("- Total Cost: $%.2f\n", ps.TotalCostUSD)
		fmt.Printf("- Total Tokens: %s input, %s output\n", formatTokens(ps.TotalInputTok), formatTokens(ps.TotalOutputTok))
		fmt.Printf("- Total Duration: %s\n", formatDuration(ps.TotalDuration))
		fmt.Printf("- Average Cost/Run: $%.2f\n", ps.TotalCostUSD/float64(ps.TotalRuns))
		successRate := float64(ps.SuccessCount) / float64(ps.TotalRuns) * 100
		fmt.Printf("- Success Rate: %.0f%%\n", successRate)
		fmt.Println()
	} else {
		fmt.Println("## Aggregate Statistics")
		fmt.Println("- No runs recorded yet.")
		fmt.Println()
	}

	if len(ps.ToolCalls) > 0 {
		fmt.Println("## Tool Usage Patterns")
		type toolCount struct {
			name  string
			count int
		}
		var sorted []toolCount
		for toolName, count := range ps.ToolCalls {
			sorted = append(sorted, toolCount{name: toolName, count: count})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		for _, tc := range sorted {
			fmt.Printf("- %s: %d calls\n", tc.name, tc.count)
		}
		fmt.Println()
	}

	turns, _ := s.ListTurns()
	profileTurns := filterTurnsByProfile(turns, name, prof.Agent)
	if len(profileTurns) > 0 {
		fmt.Println("## Turn History")
		fmt.Println()

		start := 0
		if len(profileTurns) > 20 {
			start = len(profileTurns) - 20
		}

		for _, turn := range profileTurns[start:] {
			metrics, _ := stats.ExtractFromRecording(s, turn.ID)

			outcome := "unknown"
			if turn.BuildState == "success" {
				outcome = "success"
			} else if turn.BuildState != "" {
				outcome = turn.BuildState
			}

			fmt.Printf("### Turn #%d (%s, %s, %s)\n",
				turn.ID,
				turn.Date.Format("2006-01-02"),
				formatDuration(turn.DurationSecs),
				outcome)

			if turn.Objective != "" {
				obj := turn.Objective
				if len(obj) > 200 {
					obj = obj[:200] + "..."
				}
				fmt.Printf("Objective: %s\n", obj)
			}
			if turn.WhatWasBuilt != "" {
				fmt.Printf("Outcome: %s\n", turn.WhatWasBuilt)
			}

			if metrics != nil {
				fmt.Printf("Cost: $%.4f, Tokens: %s in / %s out\n",
					metrics.TotalCostUSD,
					formatTokens(metrics.InputTokens),
					formatTokens(metrics.OutputTokens))

				if len(metrics.ToolCalls) > 0 {
					var parts []string
					for tool, count := range metrics.ToolCalls {
						parts = append(parts, fmt.Sprintf("%s(%d)", tool, count))
					}
					fmt.Printf("Tools: %s\n", strings.Join(parts, ", "))
				}
			}

			if turn.CommitHash != "" {
				fmt.Printf("Git commit: %s\n", turn.CommitHash)
			}
			if turn.KnownIssues != "" {
				fmt.Printf("Issues: %s\n", turn.KnownIssues)
			}
			fmt.Println()
		}
	}

	spawns, _ := s.ListSpawns()
	if len(spawns) > 0 {
		childCounts := make(map[string]int)
		parentCounts := make(map[string]int)
		for _, sp := range spawns {
			if sp.ParentProfile == name {
				childCounts[sp.ChildProfile]++
			}
			if sp.ChildProfile == name {
				parentCounts[sp.ParentProfile]++
			}
		}

		if len(childCounts) > 0 || len(parentCounts) > 0 {
			fmt.Println("## Spawn Relationships")
			for child, count := range childCounts {
				fmt.Printf("- Spawned %s %d times\n", child, count)
			}
			for parent, count := range parentCounts {
				fmt.Printf("- Was spawned by %s %d times\n", parent, count)
			}
			fmt.Println()
		}
	}

	return nil
}

func runStatsLoopMarkdown(s *store.Store, name string) error {
	globalCfg, _ := config.Load()
	if globalCfg == nil {
		return fmt.Errorf("global config not found")
	}

	loopDef := globalCfg.FindLoop(name)
	if loopDef == nil {
		return fmt.Errorf("loop %q not found", name)
	}

	ls, err := s.GetLoopStats(name)
	if err != nil {
		return err
	}

	fmt.Printf("# Loop Report: %s\n\n", name)

	fmt.Println("## Configuration")
	fmt.Printf("- Steps: %d\n", len(loopDef.Steps))
	for i, step := range loopDef.Steps {
		turns := step.Turns
		if turns == 0 {
			turns = 1
		}
		fmt.Printf("  %d. %s (turns: %d", i+1, step.Profile, turns)
		if step.CanStop {
			fmt.Print(", can_stop")
		}
		if step.CanMessage {
			fmt.Print(", can_message")
		}
		fmt.Println(")")
		if step.Instructions != "" {
			fmt.Printf("     Instructions: %s\n", step.Instructions)
		}
	}
	fmt.Println()

	if ls.TotalRuns > 0 {
		fmt.Println("## Aggregate Statistics")
		fmt.Printf("- Total Runs: %d\n", ls.TotalRuns)
		fmt.Printf("- Total Cycles: %d\n", ls.TotalCycles)
		fmt.Printf("- Total Cost: $%.2f\n", ls.TotalCostUSD)
		fmt.Printf("- Total Duration: %s\n", formatDuration(ls.TotalDuration))
		fmt.Printf("- Average Cost/Run: $%.2f\n", ls.TotalCostUSD/float64(ls.TotalRuns))
		fmt.Printf("- Average Cycles/Run: %.1f\n", float64(ls.TotalCycles)/float64(ls.TotalRuns))
		fmt.Println()
	} else {
		fmt.Println("## Aggregate Statistics")
		fmt.Println("- No runs recorded yet.")
		fmt.Println()
	}

	if len(ls.StepStats) > 0 {
		fmt.Println("## Per-Step Breakdown")
		for profile, count := range ls.StepStats {
			fmt.Printf("- %s: %d total runs\n", profile, count)
		}
		fmt.Println()
	}

	if len(ls.TurnIDs) > 0 {
		fmt.Println("## Turn History")
		recent := ls.TurnIDs
		if len(recent) > 20 {
			recent = recent[len(recent)-20:]
		}
		for _, tid := range recent {
			turn, err := s.GetTurn(tid)
			if err != nil {
				continue
			}
			metrics, _ := stats.ExtractFromRecording(s, tid)

			outcome := "unknown"
			if turn.BuildState == "success" {
				outcome = "success"
			} else if turn.BuildState != "" {
				outcome = turn.BuildState
			}

			fmt.Printf("- Turn #%d (%s, %s, %s)",
				tid,
				turn.Date.Format("2006-01-02"),
				formatDuration(turn.DurationSecs),
				outcome)

			if metrics != nil && metrics.TotalCostUSD > 0 {
				fmt.Printf(" cost=$%.4f", metrics.TotalCostUSD)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	return nil
}

func filterTurnsByProfile(turns []store.Turn, profileName, agentName string) []store.Turn {
	var result []store.Turn
	for _, t := range turns {
		if t.ProfileName == profileName {
			result = append(result, t)
		} else if t.ProfileName == "" && t.Agent == agentName {
			result = append(result, t)
		}
	}
	return result
}

func runStatsMigrate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	s.EnsureDirs()

	globalCfg, _ := config.Load()

	turns, err := s.ListTurns()
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}

	fmt.Printf("Migrating stats from %d turn records...\n", len(turns))

	// Build a map of agent name -> profile name for fallback matching.
	agentToProfile := make(map[string]string)
	if globalCfg != nil {
		for _, p := range globalCfg.Profiles {
			agentToProfile[p.Agent] = p.Name
		}
	}

	// Accumulate stats per profile.
	profileStatsMap := make(map[string]*store.ProfileStats)

	for _, turn := range turns {
		profileName := turn.ProfileName
		if profileName == "" {
			// Fallback: try to match agent name to a known profile.
			if pn, ok := agentToProfile[turn.Agent]; ok {
				profileName = pn
			} else {
				profileName = turn.Agent // Use agent name as last resort.
			}
		}

		ps, ok := profileStatsMap[profileName]
		if !ok {
			ps = &store.ProfileStats{
				ProfileName: profileName,
				ToolCalls:   make(map[string]int),
				SpawnedBy:   make(map[string]int),
			}
			profileStatsMap[profileName] = ps
		}

		ps.TotalRuns++
		ps.TotalDuration += turn.DurationSecs
		ps.TurnIDs = append(ps.TurnIDs, turn.ID)

		if turn.BuildState == "success" {
			ps.SuccessCount++
		} else if turn.BuildState != "" {
			ps.FailureCount++
		}

		if !turn.Date.IsZero() && turn.Date.After(ps.LastRunAt) {
			ps.LastRunAt = turn.Date
		}

		// Extract detailed metrics from recording.
		metrics, err := stats.ExtractFromRecording(s, turn.ID)
		if err == nil {
			ps.TotalCostUSD += metrics.TotalCostUSD
			ps.TotalInputTok += metrics.InputTokens
			ps.TotalOutputTok += metrics.OutputTokens
			ps.TotalTurns += metrics.NumTurns
			for tool, count := range metrics.ToolCalls {
				ps.ToolCalls[tool] += count
			}
		}
	}

	// Update spawn stats and save.
	for _, ps := range profileStatsMap {
		ps.UpdatedAt = time.Now().UTC()

		// Update spawn stats.
		spawns, _ := s.ListSpawns()
		spawnsCreated := 0
		spawnedBy := make(map[string]int)
		for _, sp := range spawns {
			if sp.ParentProfile == ps.ProfileName {
				spawnsCreated++
			}
			if sp.ChildProfile == ps.ProfileName && sp.ParentProfile != "" {
				spawnedBy[sp.ParentProfile]++
			}
		}
		ps.SpawnsCreated = spawnsCreated
		ps.SpawnedBy = spawnedBy

		if err := s.SaveProfileStats(ps); err != nil {
			fmt.Printf("  warning: failed to save stats for %s: %v\n", ps.ProfileName, err)
		} else {
			fmt.Printf("  %s%s%s: %d runs, $%.2f\n", styleBoldWhite, ps.ProfileName, colorReset, ps.TotalRuns, ps.TotalCostUSD)
		}
	}

	// Migrate loop stats.
	migrateLoopStats(s)

	fmt.Println(styleBoldGreen + "Migration complete." + colorReset)
	return nil
}

// migrateLoopStats scans loop runs and aggregates stats per loop name.
func migrateLoopStats(s *store.Store) {
	dir := s.Root()
	_ = dir // use RecordsDirs or direct loop run scanning

	// List all loop runs by scanning the loopruns directory.
	loopRunIDs := listLoopRunIDs(s)
	if len(loopRunIDs) == 0 {
		return
	}

	loopStatsMap := make(map[string]*store.LoopStats)

	for _, runID := range loopRunIDs {
		run, err := s.GetLoopRun(runID)
		if err != nil {
			continue
		}

		ls, ok := loopStatsMap[run.LoopName]
		if !ok {
			ls = &store.LoopStats{
				LoopName:  run.LoopName,
				StepStats: make(map[string]int),
			}
			loopStatsMap[run.LoopName] = ls
		}

		ls.TotalRuns++
		ls.TotalCycles += run.Cycle + 1

		for _, sid := range run.TurnIDs {
			metrics, err := stats.ExtractFromRecording(s, sid)
			if err == nil {
				ls.TotalCostUSD += metrics.TotalCostUSD
				ls.TotalDuration += metrics.DurationSecs
			}
		}

		for _, step := range run.Steps {
			ls.StepStats[step.Profile]++
		}

		ls.TurnIDs = append(ls.TurnIDs, run.TurnIDs...)

		if !run.StartedAt.IsZero() && run.StartedAt.After(ls.LastRunAt) {
			ls.LastRunAt = run.StartedAt
		}
	}

	for _, ls := range loopStatsMap {
		ls.UpdatedAt = time.Now().UTC()
		if err := s.SaveLoopStats(ls); err != nil {
			fmt.Printf("  warning: failed to save loop stats for %s: %v\n", ls.LoopName, err)
		} else {
			fmt.Printf("  %s%s%s (loop): %d runs, %d cycles, $%.2f\n",
				styleBoldWhite, ls.LoopName, colorReset, ls.TotalRuns, ls.TotalCycles, ls.TotalCostUSD)
		}
	}
}

// listLoopRunIDs returns all loop run IDs from the loopruns directory.
func listLoopRunIDs(s *store.Store) []int {
	// Try to read from the store by scanning all IDs.
	var ids []int
	for id := 1; id <= 10000; id++ {
		run, err := s.GetLoopRun(id)
		if err != nil {
			break // Assume sequential IDs; stop at first gap.
		}
		_ = run
		ids = append(ids, id)
	}
	return ids
}

// --- Formatting helpers ---

func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd ago", days)
	}
}

func formatTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func formatDuration(secs int) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm %ds", secs/60, secs%60)
	}
	hours := secs / 3600
	mins := (secs % 3600) / 60
	return fmt.Sprintf("%dh %dm", hours, mins)
}

func formatTopTools(tools map[string]int) string {
	type toolCount struct {
		name  string
		count int
	}
	var sorted []toolCount
	for name, count := range tools {
		sorted = append(sorted, toolCount{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

	var parts []string
	for i, tc := range sorted {
		if i >= 5 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%d)", tc.name, tc.count))
	}
	return strings.Join(parts, " ")
}
