package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:     "agents",
	Aliases: []string{"agent"},
	Short: "Manage agent tool configuration",
	Long: `Detect, list, configure, and health-check AI agent tools.

adaf wraps existing CLI tools (claude, codex, vibe, opencode, gemini) and
needs to know where they are installed. Use 'agents detect' to scan your
PATH and cache the results. Use 'agents set-model' to set default models.

Examples:
  adaf agents                             # List detected agents
  adaf agents detect                      # Scan PATH for agents
  adaf agents set-model claude sonnet     # Set default model
  adaf agents set-model claude opus --global
  adaf agents test claude                 # Run health check`,
	RunE: runAgentsList,
}

var agentsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List known agent tools (from cache)",
	RunE:    runAgentsList,
}

var agentsDetectCmd = &cobra.Command{
	Use:     "detect",
	Aliases: []string{"scan", "discover", "refresh"},
	Short:   "Scan PATH for agent tools and update the cache",
	RunE:    runAgentsDetect,
}

var agentsSetModelCmd = &cobra.Command{
	Use:     "set-model <agent> <model>",
	Aliases: []string{"set_model", "setmodel", "model"},
	Short:   "Set default model for an agent",
	Args:    cobra.ExactArgs(2),
	RunE:    runAgentsSetModel,
}

var agentsTestCmd = &cobra.Command{
	Use:     "test <agent>",
	Aliases: []string{"check", "health-check", "healthcheck", "health_check"},
	Short:   "Run a health-check prompt against an agent",
	Args:    cobra.ExactArgs(1),
	RunE:    runAgentsTest,
}

func init() {
	agentsTestCmd.Flags().Duration("timeout", 30*time.Second, "Health-check timeout")
	agentsSetModelCmd.Flags().Bool("global", false, "Write override to global config (~/.adaf/config.json) instead of project")

	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsDetectCmd)
	agentsCmd.AddCommand(agentsSetModelCmd)
	agentsCmd.AddCommand(agentsTestCmd)
	rootCmd.AddCommand(agentsCmd)
}

func runAgentsList(cmd *cobra.Command, args []string) error {
	adafRoot, err := agentsRoot()
	if err != nil {
		return err
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	cfg, err := agent.LoadAgentsConfig(adafRoot)
	if err != nil {
		return fmt.Errorf("loading agents config: %w", err)
	}

	rows := make([][]string, 0, len(cfg.Agents))
	names := make([]string, 0, len(cfg.Agents))
	for name, rec := range cfg.Agents {
		if !rec.Detected {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		rec := cfg.Agents[name]
		defaultModel := agent.ResolveDefaultModel(cfg, globalCfg, name)
		if defaultModel == "" {
			defaultModel = "-"
		}

		// Annotate override source.
		modelSource := ""
		if strings.TrimSpace(rec.ModelOverride) != "" {
			modelSource = " (project)"
		} else if ga, ok := globalCfg.Agents[name]; ok && strings.TrimSpace(ga.ModelOverride) != "" {
			modelSource = " (global)"
		}

		models := "-"
		if len(rec.SupportedModels) > 0 {
			models = strings.Join(rec.SupportedModels, ", ")
		}
		version := rec.Version
		if version == "" {
			version = "unknown"
		}

		rows = append(rows, []string{
			name,
			version,
			truncate(rec.Path, 56),
			defaultModel + modelSource,
			truncate(models, 64),
		})
	}

	printHeader("Agents")
	if len(rows) == 0 {
		fmt.Printf("  %sNo agents detected. Run %sadaf agents detect%s to scan.%s\n",
			colorDim, styleBoldWhite, colorDim, colorReset)
	} else {
		printTable([]string{"NAME", "VERSION", "PATH", "DEFAULT MODEL", "AVAILABLE MODELS"}, rows)
	}
	fmt.Println()
	printField("Config (project)", agent.AgentsConfigPath(adafRoot))
	printField("Config (global)", config.Dir()+"/config.json")
	printField("Detected", fmt.Sprintf("%d", len(rows)))

	return nil
}

func runAgentsDetect(cmd *cobra.Command, args []string) error {
	adafRoot, err := agentsRoot()
	if err != nil {
		return err
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	fmt.Printf("\n  %sScanning for agent tools...%s\n\n", styleBoldCyan, colorReset)

	cfg, err := agent.LoadAndSyncAgentsConfig(adafRoot, globalCfg)
	if err != nil {
		return fmt.Errorf("scanning agents: %w", err)
	}

	// Also update the in-process registry.
	agent.PopulateFromConfig(cfg)

	count := 0
	names := make([]string, 0, len(cfg.Agents))
	for name, rec := range cfg.Agents {
		if rec.Detected {
			names = append(names, name)
			count++
		}
	}
	sort.Strings(names)

	for _, name := range names {
		rec := cfg.Agents[name]
		version := rec.Version
		if version == "" {
			version = "unknown"
		}
		models := ""
		if len(rec.SupportedModels) > 0 {
			models = strings.Join(rec.SupportedModels, ", ")
		}
		fmt.Printf("  %s%s%s  %s  %s\n",
			styleBoldGreen, name, colorReset,
			colorDim+version+colorReset,
			colorDim+rec.Path+colorReset)
		if models != "" {
			fmt.Printf("    %smodels: %s%s\n", colorDim, models, colorReset)
		}
		if len(rec.ReasoningLevels) > 0 {
			var lvlNames []string
			for _, l := range rec.ReasoningLevels {
				lvlNames = append(lvlNames, l.Name)
			}
			fmt.Printf("    %sreasoning: %s%s\n", colorDim, strings.Join(lvlNames, ", "), colorReset)
		}
	}

	fmt.Printf("\n  %s%d agent(s) detected and cached.%s\n\n", styleBoldGreen, count, colorReset)
	return nil
}

func runAgentsSetModel(cmd *cobra.Command, args []string) error {
	agentName := strings.ToLower(strings.TrimSpace(args[0]))
	model := strings.TrimSpace(args[1])
	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}

	useGlobal, _ := cmd.Flags().GetBool("global")

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	if useGlobal {
		// Validate agent name.
		if _, ok := agent.Get(agentName); !ok {
			return fmt.Errorf("unknown agent %q", agentName)
		}

		ga := globalCfg.Agents[agentName]
		ga.ModelOverride = model
		globalCfg.Agents[agentName] = ga

		if err := config.Save(globalCfg); err != nil {
			return fmt.Errorf("saving global config: %w", err)
		}

		cfgPath := config.Dir() + "/config.json"
		fmt.Printf("\n  %sGlobal default model updated.%s\n\n", styleBoldGreen, colorReset)
		printField("Agent", agentName)
		printField("Model", model)
		printField("Config", cfgPath)
		return nil
	}

	adafRoot, err := agentsRoot()
	if err != nil {
		return err
	}

	cfg, err := agent.LoadAgentsConfig(adafRoot)
	if err != nil {
		return fmt.Errorf("loading agents config: %w", err)
	}

	rec, exists := cfg.Agents[agentName]
	if !exists {
		if _, ok := agent.Get(agentName); !ok {
			return fmt.Errorf("unknown agent %q", agentName)
		}
	}

	if exists && len(rec.SupportedModels) > 0 && !agent.IsModelSupported(agentName, model) {
		return fmt.Errorf("model %q is not in known models for %s (%s)", model, agentName, strings.Join(rec.SupportedModels, ", "))
	}

	cfg, err = agent.SetModelOverride(adafRoot, agentName, model, globalCfg)
	if err != nil {
		return fmt.Errorf("saving agent config: %w", err)
	}

	resolved := agent.ResolveDefaultModel(cfg, globalCfg, agentName)
	fmt.Printf("\n  %sDefault model updated.%s\n\n", styleBoldGreen, colorReset)
	printField("Agent", agentName)
	printField("Model", resolved)
	printField("Config", agent.AgentsConfigPath(adafRoot))
	return nil
}

func runAgentsTest(cmd *cobra.Command, args []string) error {
	agentName := strings.ToLower(strings.TrimSpace(args[0]))
	timeout, _ := cmd.Flags().GetDuration("timeout")

	adafRoot, err := agentsRoot()
	if err != nil {
		return err
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	cfg, err := agent.LoadAgentsConfig(adafRoot)
	if err != nil {
		return fmt.Errorf("loading agents config: %w", err)
	}

	runner, ok := agent.Get(agentName)
	if !ok {
		return fmt.Errorf("unknown agent %q", agentName)
	}

	rec := cfg.Agents[agentName]
	command := strings.TrimSpace(rec.Path)
	if command == "" {
		command = agentName
	}

	defaultModel := agent.ResolveDefaultModel(cfg, globalCfg, agentName)
	modelOverride := agent.ResolveModelOverride(cfg, globalCfg, agentName)
	runArgs := healthCheckArgs(agentName, modelOverride)

	workDir, err := os.Getwd()
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "adaf-agent-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	s, err := store.New(tmp)
	if err != nil {
		return err
	}
	recorder := recording.New(1, s)

	testPrompt := "ADAF health-check: reply with OK and exit."
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	fmt.Printf("\n  %sRunning health-check...%s\n", styleBoldCyan, colorReset)
	printField("Agent", agentName)
	if defaultModel != "" {
		printField("Default Model", defaultModel)
	}
	printField("Command", command)
	fmt.Println()

	result, runErr := runner.Run(ctx, agent.Config{
		Name:    agentName,
		Command: command,
		Args:    runArgs,
		WorkDir: workDir,
		Prompt:  testPrompt,
	}, recorder)
	if runErr != nil {
		return fmt.Errorf("health-check failed: %w", runErr)
	}
	if result == nil {
		return fmt.Errorf("health-check failed: no result")
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("health-check failed: exit code %d", result.ExitCode)
	}

	fmt.Printf("  %sHealth-check passed.%s\n\n", styleBoldGreen, colorReset)
	printField("Exit Code", fmt.Sprintf("%d", result.ExitCode))
	printField("Duration", result.Duration.Round(time.Millisecond).String())
	if out := strings.TrimSpace(firstLine(result.Output)); out != "" {
		printField("Output", truncate(out, 100))
	}
	return nil
}

func healthCheckArgs(agentName, modelOverride string) []string {
	args := make([]string, 0, 4)
	switch agentName {
	case "claude":
		if modelOverride != "" {
			args = append(args, "--model", modelOverride)
		}
		args = append(args, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			args = append(args, "--model", modelOverride)
		}
		args = append(args, "--full-auto")
	case "opencode":
		if modelOverride != "" {
			args = append(args, "--model", modelOverride)
		}
	case "gemini":
		if modelOverride != "" {
			args = append(args, "--model", modelOverride)
		}
		args = append(args, "-y")
	}
	return args
}

func agentsRoot() (string, error) {
	s, err := openStore()
	if err != nil {
		return "", err
	}
	return s.Root(), nil
}
