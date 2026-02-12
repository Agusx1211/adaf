package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/worktree"
)

var cleanupMaxAge time.Duration

var cleanupCmd = &cobra.Command{
	Use:     "cleanup",
	Aliases: []string{"gc"},
	Short:   "Remove stale worktrees and other leftover state",
	Long: `Clean up stale worktrees left behind by crashed or stopped sessions.

By default removes worktrees belonging to completed/failed/merged/rejected spawns
and any worktree older than --max-age (default 24h).

Use --max-age=0 to remove ALL adaf-managed worktrees regardless of age.`,
	RunE: runCleanup,
}

func init() {
	cleanupCmd.Flags().DurationVar(&cleanupMaxAge, "max-age", 24*time.Hour,
		"remove worktrees older than this duration (0 = remove all)")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	projCfg, err := s.LoadProject()
	if err != nil {
		return err
	}
	repoRoot := projCfg.RepoPath
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	mgr := worktree.NewManager(repoRoot)
	ctx := context.Background()

	if cleanupMaxAge == 0 {
		// Remove everything.
		if err := mgr.CleanupAll(ctx); err != nil {
			return fmt.Errorf("cleanup failed: %w", err)
		}
		fmt.Println("All adaf worktrees removed.")
		return nil
	}

	// Build dead paths from terminal spawn records.
	deadPaths := deadWorktreePaths(s)

	removed, err := mgr.CleanupStale(ctx, cleanupMaxAge, deadPaths)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	if removed == 0 {
		fmt.Println("No stale worktrees found.")
	} else {
		fmt.Printf("Removed %d stale worktree(s).\n", removed)
	}
	return nil
}

// deadWorktreePaths returns worktree paths from spawn records in terminal states.
func deadWorktreePaths(s *store.Store) map[string]bool {
	spawns, _ := s.ListSpawns()
	dead := make(map[string]bool)
	for _, rec := range spawns {
		if rec.WorktreePath == "" {
			continue
		}
		switch rec.Status {
		case "completed", "failed", "merged", "rejected":
			dead[rec.WorktreePath] = true
		}
	}
	return dead
}

