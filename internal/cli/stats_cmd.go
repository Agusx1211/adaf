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

	statsCmd.AddCommand(statsProfileCmd, statsLoopCmd, statsMigrateCmd)
	rootCmd.AddCommand(statsCmd)
}

func runStatsOverview(cmd *cobra.Command, args []string) error {
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

	// Recent sessions
	if len(ps.SessionIDs) > 0 {
		recent := ps.SessionIDs
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		var ids []string
		for _, id := range recent {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		printField("Recent Sessions", strings.Join(ids, ", "))
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

	if len(ls.SessionIDs) > 0 {
		recent := ls.SessionIDs
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}
		var ids []string
		for _, id := range recent {
			ids = append(ids, fmt.Sprintf("#%d", id))
		}
		printField("Recent Sessions", strings.Join(ids, ", "))
	}

	if !ls.LastRunAt.IsZero() {
		printField("Last Run", formatTimeAgo(ls.LastRunAt))
	}

	return nil
}

func runStatsMigrate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	s.EnsureDirs()

	globalCfg, _ := config.Load()

	logs, err := s.ListLogs()
	if err != nil {
		return fmt.Errorf("listing logs: %w", err)
	}

	fmt.Printf("Migrating stats from %d session logs...\n", len(logs))

	// Build a map of agent name -> profile name for fallback matching.
	agentToProfile := make(map[string]string)
	if globalCfg != nil {
		for _, p := range globalCfg.Profiles {
			agentToProfile[p.Agent] = p.Name
		}
	}

	// Accumulate stats per profile.
	profileStatsMap := make(map[string]*store.ProfileStats)

	for _, log := range logs {
		profileName := log.ProfileName
		if profileName == "" {
			// Fallback: try to match agent name to a known profile.
			if pn, ok := agentToProfile[log.Agent]; ok {
				profileName = pn
			} else {
				profileName = log.Agent // Use agent name as last resort.
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
		ps.TotalDuration += log.DurationSecs
		ps.SessionIDs = append(ps.SessionIDs, log.ID)

		if log.BuildState == "success" {
			ps.SuccessCount++
		} else if log.BuildState != "" {
			ps.FailureCount++
		}

		if !log.Date.IsZero() && log.Date.After(ps.LastRunAt) {
			ps.LastRunAt = log.Date
		}

		// Extract detailed metrics from recording.
		metrics, err := stats.ExtractFromRecording(s, log.ID)
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

		for _, sid := range run.SessionIDs {
			metrics, err := stats.ExtractFromRecording(s, sid)
			if err == nil {
				ls.TotalCostUSD += metrics.TotalCostUSD
				ls.TotalDuration += metrics.DurationSecs
			}
		}

		for _, step := range run.Steps {
			ls.StepStats[step.Profile]++
		}

		ls.SessionIDs = append(ls.SessionIDs, run.SessionIDs...)

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
