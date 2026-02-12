package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/loop"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

var runCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"execute", "exec"},
	Short:   "Run an agent loop against the project (inline output for CI/scripts)",
	Long: `Run an AI agent against the project. The agent receives a prompt built from
the current project context (plan, issues, decisions, session history) and
works autonomously for the specified number of turns.

Output is printed inline (suitable for CI/pipes). For the interactive TUI
with real-time monitoring, run 'adaf' with no subcommand.

Supported agents: claude, codex, vibe, opencode, gemini, generic

Examples:
  # Run claude for a single turn
  adaf run --agent claude --max-turns 1

  # Run with a custom prompt
  adaf run --agent claude --prompt "Fix the failing tests in auth/"

  # Run codex with a specific model
  adaf run --agent codex --model gpt-5.1-codex-max

  # Run as a detachable session (like tmux)
  adaf run --agent claude -s
  adaf attach <session-id>

  # Run with extended reasoning
  adaf run --agent claude --reasoning-level high`,
	RunE: runAgent,
}

func init() {
	runCmd.Flags().String("agent", "claude", "Agent to run (claude, codex, vibe, opencode, generic)")
	runCmd.Flags().String("prompt", "", "Prompt to send to the agent (default: built from project context)")
	runCmd.Flags().Int("max-turns", 0, "Maximum number of agent turns (0 = unlimited)")
	runCmd.Flags().String("model", "", "Model override for the agent")
	runCmd.Flags().String("command", "", "Custom command path (for generic agent)")
	runCmd.Flags().String("reasoning-level", "", "Reasoning level (e.g. low, medium, high, xhigh)")
	runCmd.Flags().BoolP("session", "s", false, "Run as a detachable session (use 'adaf attach' to reattach)")
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
	reasoningLevel, _ := cmd.Flags().GetString("reasoning-level")
	modelFlag = strings.TrimSpace(modelFlag)
	reasoningLevel = strings.TrimSpace(reasoningLevel)

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
		case "claude", "codex", "vibe", "opencode", "gemini", "generic":
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

	var promptBuildOpts *promptpkg.BuildOpts

	// If no explicit prompt was provided, build one from project context.
	if prompt == "" {
		opts := promptpkg.BuildOpts{
			Store:   s,
			Project: projCfg,
		}
		built, err := promptpkg.Build(opts)
		if err != nil {
			return fmt.Errorf("building default prompt: %w", err)
		}
		prompt = built
		promptBuildOpts = &opts
	}

	// Build agent args based on agent type.
	var agentArgs []string
	agentEnv := make(map[string]string)
	switch agentName {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentEnv["CLAUDE_CODE_EFFORT_LEVEL"] = reasoningLevel
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentArgs = append(agentArgs, "-c", `model_reasoning_effort="`+reasoningLevel+`"`)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "vibe":
		// vibe reads prompt from cfg.Prompt via stdin, no extra args needed.
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	case "gemini":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "-y")
	}

	agentCfg := agent.Config{
		Name:     agentName,
		Command:  customCmd,
		Args:     agentArgs,
		Env:      agentEnv,
		WorkDir:  workDir,
		Prompt:   prompt,
		MaxTurns: maxTurns,
	}

	sessionMode, _ := cmd.Flags().GetBool("session")
	if sessionMode {
		if session.IsAgentContext() {
			return fmt.Errorf("session mode is not available inside an agent context")
		}
		return runAsSession(agentName, agentCfg, projCfg, workDir, promptBuildOpts != nil)
	}

	// Default: inline output.
	return runInline(cmd, s, agentInstance, agentCfg, projCfg, defaultModel, maxTurns, promptBuildOpts)
}

// runAsSession starts the agent as a background session daemon and prints the session ID.
func runAsSession(agentName string, agentCfg agent.Config, projCfg *store.ProjectConfig, workDir string, useDefaultPrompt bool) error {
	dcfg := session.DaemonConfig{
		AgentName:        agentName,
		AgentCommand:     agentCfg.Command,
		AgentArgs:        agentCfg.Args,
		AgentEnv:         agentCfg.Env,
		WorkDir:          workDir,
		Prompt:           agentCfg.Prompt,
		MaxTurns:         agentCfg.MaxTurns,
		ProjectDir:       workDir,
		ProfileName:      agentName,
		ProjectName:      projCfg.Name,
		UseDefaultPrompt: useDefaultPrompt,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	if err := session.StartDaemon(sessionID); err != nil {
		return fmt.Errorf("starting session daemon: %w", err)
	}

	fmt.Printf("\n  %sSession #%d started%s (agent=%s, project=%s)\n",
		styleBoldGreen, sessionID, colorReset, agentName, projCfg.Name)
	fmt.Printf("  Use %sadaf attach %d%s to connect.\n", styleBoldWhite, sessionID, colorReset)
	fmt.Printf("  Use %sadaf sessions%s to list all sessions.\n\n", styleBoldWhite, colorReset)

	return nil
}

// runInline prints inline output suitable for CI/pipes.
func runInline(cmd *cobra.Command, s *store.Store, agentInstance agent.Agent, agentCfg agent.Config, projCfg *store.ProjectConfig, defaultModel string, maxTurns int, promptBuildOpts *promptpkg.BuildOpts) error {
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
	if promptBuildOpts != nil {
		basePrompt := agentCfg.Prompt
		opts := *promptBuildOpts
		l.PromptFunc = func(sessionID int, supervisorNotes []store.SupervisorNote) string {
			opts.SupervisorNotes = supervisorNotes
			built, err := promptpkg.Build(opts)
			if err != nil {
				return basePrompt
			}
			return built
		}
	}

	if err := l.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
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
