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
	"github.com/agusx1211/adaf/internal/store"
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
	runCmd.Flags().String("prompt", "", "Prompt to send to the agent (default: built from project context)")
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

	// If no explicit prompt was provided, build one from project context.
	if prompt == "" {
		built, err := buildDefaultPrompt(s, config)
		if err != nil {
			return fmt.Errorf("building default prompt: %w", err)
		}
		prompt = built
	}

	// Build agent args based on agent type and flags.
	// NOTE: For agents where the Run() method appends the prompt from
	// cfg.Prompt (e.g. claude), do NOT add it to agentArgs here to
	// avoid duplicating the -p flag.
	var agentArgs []string
	switch agentName {
	case "claude":
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

// buildDefaultPrompt constructs a prompt from the project's plan and open
// issues so the agent has meaningful context when no --prompt is given.
func buildDefaultPrompt(s *store.Store, config *store.ProjectConfig) (string, error) {
	var b strings.Builder

	b.WriteString("You are working on the project: " + config.Name + "\n\n")

	// Include current plan context.
	plan, err := s.LoadPlan()
	if err == nil && len(plan.Phases) > 0 {
		b.WriteString("## Current Plan\n")
		if plan.Title != "" {
			b.WriteString(plan.Title + "\n")
		}
		b.WriteString("\nPhases:\n")
		for _, p := range plan.Phases {
			b.WriteString(fmt.Sprintf("- [%s] %s: %s\n", p.Status, p.ID, p.Title))
		}
		b.WriteString("\n")

		// Find the first actionable phase to focus the agent.
		for _, p := range plan.Phases {
			if p.Status == "not_started" || p.Status == "in_progress" {
				b.WriteString("## Current Task\n")
				b.WriteString(fmt.Sprintf("Work on phase %s: %s\n\n", p.ID, p.Title))
				if p.Description != "" {
					b.WriteString(p.Description + "\n\n")
				}
				break
			}
		}
	}

	// Include open issues.
	issues, err := s.ListIssues()
	if err == nil && len(issues) > 0 {
		var open []store.Issue
		for _, iss := range issues {
			if iss.Status == "open" || iss.Status == "in_progress" {
				open = append(open, iss)
			}
		}
		if len(open) > 0 {
			b.WriteString("## Open Issues\n")
			for _, iss := range open {
				b.WriteString(fmt.Sprintf("- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description))
			}
			b.WriteString("\n")
		}
	}

	// Include recent session context so the agent knows what happened last.
	latest, err := s.LatestLog()
	if err == nil && latest != nil {
		b.WriteString("## Last Session\n")
		if latest.Objective != "" {
			b.WriteString(fmt.Sprintf("Objective: %s\n", latest.Objective))
		}
		if latest.WhatWasBuilt != "" {
			b.WriteString(fmt.Sprintf("Built: %s\n", latest.WhatWasBuilt))
		}
		if latest.NextSteps != "" {
			b.WriteString(fmt.Sprintf("Next steps: %s\n", latest.NextSteps))
		}
		b.WriteString("\n")
	}

	b.WriteString("Implement the current task described above. Write code, run tests, and ensure everything compiles.")

	return b.String(), nil
}
