package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:     "init",
	Aliases: []string{"initialize", "setup"},
	Short:   "Initialize a new adaf project",
	Long: `Initialize a new adaf project in the current directory (or specified directory).
	Creates a .adaf.json marker in the repo and initializes the project store
	under ~/.adaf/projects/<id> with plans, issues, wiki, turn logs, and recordings.

Also scans PATH for installed AI agent tools (claude, codex, vibe, etc.)
and caches the results for future runs.

Examples:
  # Initialize in current directory
  adaf init

  # Initialize with a custom name
  adaf init --name my-cool-project

  # Initialize for a different repo
  adaf init --repo /path/to/other/repo`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().String("name", "", "Project name (defaults to directory name)")
	initCmd.Flags().String("repo", ".", "Path to the target repository")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	repoPath, _ := cmd.Flags().GetString("repo")
	name, _ := cmd.Flags().GetString("name")

	// Resolve to absolute path
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
	}

	// Default project name to directory name
	if name == "" {
		name = filepath.Base(absRepo)
	}

	// Check if already initialized
	s, err := store.New(absRepo)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	if s.Exists() {
		created, err := s.Repair()
		if err != nil {
			return fmt.Errorf("repairing existing project: %w", err)
		}

		fmt.Println()
		fmt.Printf("  %sadaf project already exists%s\n", styleBoldCyan, colorReset)
		printField("Marker", store.ProjectMarkerPath(absRepo))
		printField("Store", s.Root())
		if len(created) == 0 {
			fmt.Printf("  %sNo repairs needed.%s\n\n", styleBoldGreen, colorReset)
			return nil
		}

		fmt.Printf("  %sRecreated %d missing director", styleBoldGreen, len(created))
		if len(created) == 1 {
			fmt.Printf("y%s\n", colorReset)
		} else {
			fmt.Printf("ies%s\n", colorReset)
		}
		for _, dir := range created {
			fmt.Printf("    %s\n", dir)
		}
		fmt.Println()
		return nil
	}

	projCfg := store.ProjectConfig{
		Name:        name,
		RepoPath:    absRepo,
		AgentConfig: make(map[string]string),
		Metadata:    make(map[string]any),
	}

	if err := s.Init(projCfg); err != nil {
		return fmt.Errorf("initializing project: %w", err)
	}

	// Run initial agent detection so the cache is populated.
	globalCfg, _ := config.Load()
	agentsCfg, scanErr := agent.LoadAndSyncAgentsConfig(globalCfg)
	if scanErr == nil {
		agent.PopulateFromConfig(agentsCfg)
	}

	fmt.Println()
	fmt.Printf("  %s adaf project initialized!%s\n", styleBoldGreen, colorReset)
	fmt.Println()
	printField("Project", name)
	printField("Project ID", s.ProjectID())
	printField("Marker", store.ProjectMarkerPath(absRepo))
	printField("Store", s.Root())
	printField("Repo", absRepo)
	fmt.Println()
	fmt.Printf("  %sCreated:%s\n", colorDim, colorReset)
	fmt.Printf("    %s\n", store.ProjectMarkerPath(absRepo))
	fmt.Printf("    %s/project.json\n", s.Root())
	fmt.Printf("    %s/plans/\n", s.Root())
	fmt.Printf("    %s/local/turns/\n", s.Root())
	fmt.Printf("    %s/issues/\n", s.Root())
	fmt.Printf("    %s/wiki/\n", s.Root())
	fmt.Printf("    %s/local/records/\n", s.Root())
	if agentsCfg != nil {
		detected := 0
		for _, rec := range agentsCfg.Agents {
			if rec.Detected {
				detected++
			}
		}
		printField("Agents found", fmt.Sprintf("%d (run 'adaf config agents' to list)", detected))
	}
	fmt.Println()
	fmt.Printf("  Next: run %sadaf plan create --id default --title \"Initial Plan\"%s.\n", styleBoldWhite, colorReset)

	// If the repo path differs from cwd, print a hint
	cwd, _ := os.Getwd()
	if cwd != absRepo {
		fmt.Printf("  Note: project was created at %s (not the current directory).\n", absRepo)
	}

	return nil
}
