package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/stats"
	"github.com/agusx1211/adaf/internal/store"
)

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

	agentToProfile := make(map[string]string)
	if globalCfg != nil {
		for _, p := range globalCfg.Profiles {
			agentToProfile[p.Agent] = p.Name
		}
	}

	profileStatsMap := make(map[string]*store.ProfileStats)

	for _, turn := range turns {
		profileName := turn.ProfileName
		if profileName == "" {
			if pn, ok := agentToProfile[turn.Agent]; ok {
				profileName = pn
			} else {
				profileName = turn.Agent
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

	for _, ps := range profileStatsMap {
		ps.UpdatedAt = time.Now().UTC()

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

	migrateLoopStats(s)

	fmt.Println(styleBoldGreen + "Migration complete." + colorReset)
	return nil
}

func migrateLoopStats(s *store.Store) {
	dir := s.Root()
	_ = dir

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

func listLoopRunIDs(s *store.Store) []int {
	var ids []int
	for id := 1; id <= 10000; id++ {
		run, err := s.GetLoopRun(id)
		if err != nil {
			break
		}
		_ = run
		ids = append(ids, id)
	}
	return ids
}
