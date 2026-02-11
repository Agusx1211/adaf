package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/loop"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an agent loop against the project",
	Long: `Run an AI agent in a loop against the project. The agent will work on the
current plan, resolve issues, and log its progress.

Supported agents: claude, codex, vibe, generic`,
	RunE: runAgent,
}

func init() {
	runCmd.Flags().String("agent", "claude", "Agent to run (claude, codex, vibe, generic)")
	runCmd.Flags().String("prompt", ".", "Prompt to send to the agent")
	runCmd.Flags().Int("max-turns", 1, "Maximum number of agent turns (0 = unlimited)")
	runCmd.Flags().String("model", "", "Model override for the agent")
	runCmd.Flags().Bool("auto-approve", false, "Auto-approve agent actions without confirmation")
	runCmd.Flags().String("command", "", "Custom command path (for generic agent)")
	rootCmd.AddCommand(runCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	agentName, _ := cmd.Flags().GetString("agent")
	prompt, _ := cmd.Flags().GetString("prompt")
	maxTurns, _ := cmd.Flags().GetInt("max-turns")
	model, _ := cmd.Flags().GetString("model")
	autoApprove, _ := cmd.Flags().GetBool("auto-approve")
	customCmd, _ := cmd.Flags().GetString("command")

	// Look up agent from registry
	agentInstance, ok := agent.Get(agentName)
	if !ok {
		return fmt.Errorf("unknown agent %q (valid: %s)", agentName, strings.Join(agentNames(), ", "))
	}

	// Load project config
	config, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	workDir := config.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// Build agent args based on agent type and flags
	var agentArgs []string
	switch agentName {
	case "claude":
		agentArgs = append(agentArgs, "-p", prompt)
		if model != "" {
			agentArgs = append(agentArgs, "--model", model)
		}
		if autoApprove {
			agentArgs = append(agentArgs, "--dangerously-skip-permissions")
		}
	case "codex":
		agentArgs = append(agentArgs, prompt)
		if model != "" {
			agentArgs = append(agentArgs, "--model", model)
		}
		if autoApprove {
			agentArgs = append(agentArgs, "--full-auto")
		}
	case "vibe":
		agentArgs = append(agentArgs, prompt)
	}

	agentCfg := agent.Config{
		Name:     agentName,
		Command:  customCmd,
		Args:     agentArgs,
		WorkDir:  workDir,
		Prompt:   prompt,
		MaxTurns: maxTurns,
	}

	// Print run header
	fmt.Println()
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println(styleBoldCyan + "   adaf agent run" + colorReset)
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println()
	printField("Project", config.Name)
	printField("Repo", workDir)
	printField("Agent", agentName)
	if model != "" {
		printField("Model", model)
	}
	printField("Prompt", prompt)
	if maxTurns > 0 {
		printField("Max Turns", fmt.Sprintf("%d", maxTurns))
	} else {
		printField("Max Turns", "unlimited")
	}
	printField("Auto-Approve", fmt.Sprintf("%v", autoApprove))
	printField("Started", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()
	fmt.Println(colorDim + "  " + strings.Repeat("-", 46) + colorReset)
	fmt.Println()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Printf("\n  %sReceived interrupt, finishing current turn...%s\n", styleBoldYellow, colorReset)
		cancel()
	}()

	// Create and run the loop
	l := &loop.Loop{
		Store:  s,
		Agent:  agentInstance,
		Config: agentCfg,
		OnStart: func(sessionID int) {
			fmt.Printf("  %s>>> Session #%d starting%s\n", styleBoldGreen, sessionID, colorReset)
		},
		OnEnd: func(sessionID int, result *agent.Result) {
			if result != nil {
				fmt.Printf("  %s<<< Session #%d completed (exit=%d, %s)%s\n",
					styleBoldGreen, sessionID, result.ExitCode, result.Duration.Round(time.Second), colorReset)
			} else {
				fmt.Printf("  %s<<< Session #%d ended%s\n", styleBoldYellow, sessionID, colorReset)
			}
		},
	}

	if err := l.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("agent loop: %w", err)
	}

	fmt.Printf("\n  %sAgent loop finished.%s\n\n", styleBoldGreen, colorReset)
	return nil
}

func agentNames() []string {
	all := agent.All()
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	return names
}
