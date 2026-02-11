package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/worktree"
)

var worktreeCmd = &cobra.Command{
	Use:     "worktree",
	Aliases: []string{"worktrees", "wt"},
	Short:   "Manage adaf git worktrees",
}

var worktreeCleanupCmd = &cobra.Command{
	Use:     "cleanup",
	Aliases: []string{"clean", "prune", "gc"},
	Short:   "Remove all adaf-managed worktrees (crash recovery)",
	RunE:    runWorktreeCleanup,
}

var worktreeListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List active adaf-managed worktrees",
	RunE:    runWorktreeList,
}

func init() {
	worktreeCmd.AddCommand(worktreeCleanupCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	rootCmd.AddCommand(worktreeCmd)
}

func runWorktreeCleanup(cmd *cobra.Command, args []string) error {
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
	if err := mgr.CleanupAll(context.Background()); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}
	fmt.Println("All adaf worktrees cleaned up.")
	return nil
}

func runWorktreeList(cmd *cobra.Command, args []string) error {
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
	active, err := mgr.ListActive(context.Background())
	if err != nil {
		return err
	}

	if len(active) == 0 {
		fmt.Println("No active adaf worktrees.")
		return nil
	}

	printHeader("Active Worktrees")
	for _, wt := range active {
		printField("Path", wt.Path)
		printField("Branch", wt.Branch)
		fmt.Println()
	}
	return nil
}
