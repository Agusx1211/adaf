package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Detect and manage agent tool configuration",
	RunE:  runAgentsList,
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List detected agent tools",
	RunE:  runAgentsList,
}

var agentsSetModelCmd = &cobra.Command{
	Use:   "set-model <agent> <model>",
	Short: "Set default model for an agent",
	Args:  cobra.ExactArgs(2),
	RunE:  runAgentsSetModel,
}

var agentsTestCmd = &cobra.Command{
	Use:   "test <agent>",
	Short: "Run a health-check prompt against an agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentsTest,
}

func init() {
	agentsTestCmd.Flags().Duration("timeout", 30*time.Second, "Health-check timeout")

	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsSetModelCmd)
	agentsCmd.AddCommand(agentsTestCmd)
	rootCmd.AddCommand(agentsCmd)
}

func runAgentsList(cmd *cobra.Command, args []string) error {
	adafRoot, err := agentsRoot()
	if err != nil {
		return err
	}

	cfg, err := agent.LoadAndSyncAgentsConfig(adafRoot)
	if err != nil {
		return fmt.Errorf("scanning agents: %w", err)
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
		defaultModel := agent.ResolveDefaultModel(cfg, name)
		if defaultModel == "" {
			defaultModel = "-"
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
			defaultModel,
			truncate(models, 64),
		})
	}

	printHeader("Detected Agents")
	printTable([]string{"NAME", "VERSION", "PATH", "DEFAULT MODEL", "AVAILABLE MODELS"}, rows)
	fmt.Println()
	printField("Config", agent.AgentsConfigPath(adafRoot))
	printField("Detected", fmt.Sprintf("%d", len(rows)))

	return nil
}

func runAgentsSetModel(cmd *cobra.Command, args []string) error {
	agentName := strings.ToLower(strings.TrimSpace(args[0]))
	model := strings.TrimSpace(args[1])
	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}

	adafRoot, err := agentsRoot()
	if err != nil {
		return err
	}

	cfg, err := agent.LoadAndSyncAgentsConfig(adafRoot)
	if err != nil {
		return fmt.Errorf("scanning agents: %w", err)
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

	cfg, err = agent.SetModelOverride(adafRoot, agentName, model)
	if err != nil {
		return fmt.Errorf("saving agent config: %w", err)
	}

	resolved := agent.ResolveDefaultModel(cfg, agentName)
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

	cfg, err := agent.LoadAndSyncAgentsConfig(adafRoot)
	if err != nil {
		return fmt.Errorf("scanning agents: %w", err)
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

	defaultModel := agent.ResolveDefaultModel(cfg, agentName)
	modelOverride := agent.ResolveModelOverride(cfg, agentName)
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
