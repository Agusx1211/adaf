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
	Use:   "init",
	Short: "Initialize a new adaf project",
	Long:  `Initialize a new adaf project in the current directory (or specified directory). Creates the .adaf/ directory structure and project.json configuration.`,
	RunE:  runInit,
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
		return fmt.Errorf("adaf project already exists at %s", absRepo)
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

	// Write a .gitignore inside .adaf/ to keep recordings out of version control.
	gitignorePath := filepath.Join(absRepo, store.AdafDir, ".gitignore")
	gitignoreContent := `# Raw session I/O recordings
recordings/
records/

# Machine-specific agent detection cache
agents.json

# Session logs (ephemeral per-run artifacts)
logs/
`
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return fmt.Errorf("writing .adaf/.gitignore: %w", err)
	}

	// Run initial agent detection so the cache is populated.
	globalCfg, _ := config.Load()
	agentsCfg, scanErr := agent.LoadAndSyncAgentsConfig(s.Root(), globalCfg)
	if scanErr == nil {
		agent.PopulateFromConfig(agentsCfg)
	}

	fmt.Println()
	fmt.Printf("  %s adaf project initialized!%s\n", styleBoldGreen, colorReset)
	fmt.Println()
	printField("Project", name)
	printField("Location", filepath.Join(absRepo, store.AdafDir))
	printField("Repo", absRepo)
	fmt.Println()
	fmt.Printf("  %sCreated:%s\n", colorDim, colorReset)
	fmt.Printf("    %s/.adaf/project.json\n", absRepo)
	fmt.Printf("    %s/.adaf/logs/\n", absRepo)
	fmt.Printf("    %s/.adaf/issues/\n", absRepo)
	fmt.Printf("    %s/.adaf/docs/\n", absRepo)
	fmt.Printf("    %s/.adaf/decisions/\n", absRepo)
	fmt.Printf("    %s/.adaf/recordings/\n", absRepo)
	if agentsCfg != nil {
		detected := 0
		for _, rec := range agentsCfg.Agents {
			if rec.Detected {
				detected++
			}
		}
		printField("Agents found", fmt.Sprintf("%d (run 'adaf agents' to list)", detected))
	}
	fmt.Println()
	fmt.Printf("  Next: run %sadaf plan set <plan-file>%s to set your project plan.\n", styleBoldWhite, colorReset)

	// If the repo path differs from cwd, print a hint
	cwd, _ := os.Getwd()
	if cwd != absRepo {
		fmt.Printf("  Note: project was created at %s (not the current directory).\n", absRepo)
	}

	return nil
}
