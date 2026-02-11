package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/loop"
	"github.com/agusx1211/adaf/internal/store"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run an agent loop against the project (inline output for CI/scripts)",
	Long: `Run an AI agent in a loop against the project. The agent will work on the
current plan, resolve issues, and log its progress.

Output is printed inline (suitable for CI/pipes). For the interactive TUI,
run 'adaf' with no subcommand.

Supported agents: claude, codex, vibe, opencode, generic`,
	RunE: runAgent,
}

func init() {
	runCmd.Flags().String("agent", "claude", "Agent to run (claude, codex, vibe, opencode, generic)")
	runCmd.Flags().String("prompt", "", "Prompt to send to the agent (default: built from project context)")
	runCmd.Flags().Int("max-turns", 0, "Maximum number of agent turns (0 = unlimited)")
	runCmd.Flags().String("model", "", "Model override for the agent")
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
	modelFlag, _ := cmd.Flags().GetString("model")
	customCmd, _ := cmd.Flags().GetString("command")
	modelFlag = strings.TrimSpace(modelFlag)

	// Look up agent from registry.
	agentInstance, ok := agent.Get(agentName)
	if !ok {
		return fmt.Errorf("unknown agent %q (valid: %s)", agentName, strings.Join(agentNames(), ", "))
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	agentsCfg, err := agent.LoadAgentsConfig(s.Root())
	if err != nil {
		return fmt.Errorf("loading agent configuration: %w", err)
	}
	if rec, ok := agentsCfg.Agents[agentName]; ok {
		if customCmd == "" && rec.Path != "" {
			customCmd = rec.Path
		}
	}
	defaultModel := agent.ResolveDefaultModel(agentsCfg, globalCfg, agentName)
	modelOverride := agent.ResolveModelOverride(agentsCfg, globalCfg, agentName)
	if modelFlag != "" {
		modelOverride = modelFlag
		defaultModel = modelFlag
	}
	if customCmd == "" {
		switch agentName {
		case "claude", "codex", "vibe", "opencode", "generic":
		default:
			customCmd = agentName
		}
	}

	// Load project config
	projCfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// If no explicit prompt was provided, build one from project context.
	if prompt == "" {
		built, err := buildDefaultPrompt(s, projCfg)
		if err != nil {
			return fmt.Errorf("building default prompt: %w", err)
		}
		prompt = built
	}

	// Build agent args based on agent type.
	var agentArgs []string
	switch agentName {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "vibe":
		// vibe reads prompt from cfg.Prompt via stdin, no extra args needed.
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	}

	agentCfg := agent.Config{
		Name:     agentName,
		Command:  customCmd,
		Args:     agentArgs,
		WorkDir:  workDir,
		Prompt:   prompt,
		MaxTurns: maxTurns,
	}

	// Always inline output.
	return runInline(cmd, s, agentInstance, agentCfg, projCfg, defaultModel, maxTurns)
}

// runInline prints inline output suitable for CI/pipes.
func runInline(cmd *cobra.Command, s *store.Store, agentInstance agent.Agent, agentCfg agent.Config, projCfg *store.ProjectConfig, defaultModel string, maxTurns int) error {
	workDir := agentCfg.WorkDir

	// Print run header
	fmt.Println()
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println(styleBoldCyan + "   adaf agent run" + colorReset)
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println()
	printField("Project", projCfg.Name)
	printField("Repo", workDir)
	printField("Agent", agentCfg.Name)
	if defaultModel != "" {
		printField("Default Model", defaultModel)
	}
	printField("Prompt", agentCfg.Prompt)
	if maxTurns > 0 {
		printField("Max Turns", fmt.Sprintf("%d", maxTurns))
	} else {
		printField("Max Turns", "unlimited")
	}
	printField("Auto-Approve", "true")
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

// maxAgentsMDSize is the upper bound (in bytes) for embedding AGENTS.md into
// the prompt. Files larger than this are referenced instead of inlined.
const maxAgentsMDSize = 16 * 1024

// buildDefaultPrompt constructs a prompt from the project's plan and open
// issues so the agent has meaningful context when no --prompt is given.
//
// The prompt is structured in priority order:
//  1. Objective — what to do right now (single phase)
//  2. Rules     — constraints the agent must follow
//  3. Context   — last session, neighboring phases, relevant issues
//  4. Reference — AGENTS.md, full plan overview (only when useful)
func buildDefaultPrompt(s *store.Store, projCfg *store.ProjectConfig) (string, error) {
	var b strings.Builder

	workDir := projCfg.RepoPath
	plan, _ := s.LoadPlan()
	latest, _ := s.LatestLog()

	b.WriteString("# Objective\n\n")
	b.WriteString("Project: " + projCfg.Name + "\n\n")

	var currentPhase *store.PlanPhase
	if plan != nil && len(plan.Phases) > 0 {
		for i := range plan.Phases {
			p := &plan.Phases[i]
			if p.Status == "not_started" || p.Status == "in_progress" {
				currentPhase = p
				break
			}
		}
	}

	if currentPhase != nil {
		fmt.Fprintf(&b, "Your task is to work on phase **%s: %s**.\n\n", currentPhase.ID, currentPhase.Title)
		if currentPhase.Description != "" {
			b.WriteString(currentPhase.Description + "\n\n")
		}
	} else if plan != nil && plan.Title != "" {
		b.WriteString("All planned phases are complete. Look for remaining open issues or improvements.\n\n")
	} else {
		b.WriteString("No plan is set. Explore the codebase and address any open issues.\n\n")
	}

	b.WriteString("# Rules\n\n")
	b.WriteString("- Write code, run tests, and ensure everything compiles before finishing.\n")
	b.WriteString("- Focus on one coherent unit of work. Stop when the current phase (or a meaningful increment of it) is complete.\n")
	b.WriteString("- Do NOT read or write files inside the `.adaf/` directory directly. " +
		"Use `adaf` CLI commands instead (`adaf issues`, `adaf log`, `adaf plan`, etc.). " +
		"The `.adaf/` directory structure may change and direct access will be restricted in the future.\n")
	b.WriteString("\n")

	b.WriteString("# Context\n\n")

	if latest != nil {
		b.WriteString("## Last Session\n")
		if latest.Objective != "" {
			fmt.Fprintf(&b, "- Objective: %s\n", latest.Objective)
		}
		if latest.WhatWasBuilt != "" {
			fmt.Fprintf(&b, "- Built: %s\n", latest.WhatWasBuilt)
		}
		if latest.NextSteps != "" {
			fmt.Fprintf(&b, "- Next steps: %s\n", latest.NextSteps)
		}
		if latest.KnownIssues != "" {
			fmt.Fprintf(&b, "- Known issues: %s\n", latest.KnownIssues)
		}
		b.WriteString("\n")
	}

	issues, err := s.ListIssues()
	if err == nil && len(issues) > 0 {
		var relevant []store.Issue
		for _, iss := range issues {
			if iss.Status != "open" && iss.Status != "in_progress" {
				continue
			}
			relevant = append(relevant, iss)
		}
		if len(relevant) > 0 {
			b.WriteString("## Open Issues\n")
			for _, iss := range relevant {
				fmt.Fprintf(&b, "- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description)
			}
			b.WriteString("\n")
		}
	}

	if currentPhase != nil && plan != nil && len(plan.Phases) > 1 {
		b.WriteString("## Neighboring Phases\n")
		for i, p := range plan.Phases {
			if p.ID == currentPhase.ID {
				if i > 0 {
					prev := plan.Phases[i-1]
					fmt.Fprintf(&b, "- Previous: [%s] %s: %s\n", prev.Status, prev.ID, prev.Title)
				}
				fmt.Fprintf(&b, "- **Current: [%s] %s: %s**\n", p.Status, p.ID, p.Title)
				if i < len(plan.Phases)-1 {
					next := plan.Phases[i+1]
					fmt.Fprintf(&b, "- Next: [%s] %s: %s\n", next.Status, next.ID, next.Title)
				}
				break
			}
		}
		b.WriteString("\n")
	}

	if workDir != "" {
		agentsMD := filepath.Join(workDir, "AGENTS.md")
		if info, err := os.Stat(agentsMD); err == nil {
			if info.Size() <= maxAgentsMDSize {
				if data, err := os.ReadFile(agentsMD); err == nil {
					b.WriteString("# AGENTS.md\n\n")
					b.WriteString("The repository includes an AGENTS.md with instructions for AI agents. Follow these:\n\n")
					b.WriteString(string(data))
					b.WriteString("\n\n")
				}
			} else {
				b.WriteString("# AGENTS.md\n\n")
				fmt.Fprintf(&b, "The repository includes an AGENTS.md file at `%s`. Read it before starting work — it contains important instructions for AI agents.\n\n", agentsMD)
			}
		}
	}

	return b.String(), nil
}
