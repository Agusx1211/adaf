package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/events"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

var askCmd = &cobra.Command{
	Use:   "ask [prompt]",
	Short: "Run a single agent session (with optional prompt) and exit",
	Long: `Run a single standalone agent session. Unlike 'run', 'ask' is designed for
one-shot usage: the agent runs once and adaf exits.

If no prompt is provided, the agent runs in standalone mode using the full
project context (rules, plan, docs, etc.) — similar to opening a vibe coding
agent directly, but with ADAF's context injection.

The prompt can be provided as:
  - A positional argument: adaf ask "Fix the failing tests"
  - Via --prompt flag: adaf ask --prompt "Fix the failing tests"
  - Via stdin pipe: echo "Fix the failing tests" | adaf ask
  - Omitted entirely for standalone mode: adaf ask --agent claude

Use --count N to repeat the same prompt N times sequentially.
When --count > 1, each run gets the same prompt but can optionally include
the previous run's output as context (--chain).

Examples:
  adaf ask --agent claude                  # standalone mode with project context
  adaf ask "Fix the failing tests in auth/"
  adaf ask --agent codex --model gpt-5.1-codex-max "Refactor the utils"
  adaf ask --count 3 "Run and fix tests until they pass"
  echo "Explain the architecture" | adaf ask --agent claude
  adaf ask -s "Long running task"`,
	RunE: runAsk,
}

func init() {
	askCmd.Flags().String("agent", "claude", "Agent to use (claude, codex, vibe, opencode, gemini, generic)")
	askCmd.Flags().String("profile", "", "Use a named profile instead of --agent/--model")
	askCmd.Flags().String("prompt", "", "Prompt/instructions (alternative to positional arg)")
	askCmd.Flags().String("model", "", "Model override for the agent")
	askCmd.Flags().String("command", "", "Custom command path for the selected agent")
	askCmd.Flags().String("reasoning-level", "", "Reasoning level (e.g. low, medium, high, xhigh)")
	askCmd.Flags().String("plan", "", "Plan ID override (defaults to active plan)")
	askCmd.Flags().Int("count", 1, "Number of sequential runs (default 1)")
	askCmd.Flags().Bool("chain", false, "When --count > 1, include previous run's output as context")
	askCmd.Flags().BoolP("session", "s", false, "Start detached (use 'adaf attach' to connect)")
	rootCmd.AddCommand(askCmd)
}

func runAsk(cmd *cobra.Command, args []string) error {
	debug.Log("cli.ask", "runAsk() called")
	if session.IsAgentContext() {
		return fmt.Errorf("ask is not available inside an agent context")
	}

	// Resolve the prompt from args, --prompt flag, or stdin.
	prompt, err := resolveAskPrompt(cmd, args)
	if err != nil {
		return err
	}

	count, _ := cmd.Flags().GetInt("count")
	if count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	chain, _ := cmd.Flags().GetBool("chain")
	sessionMode, _ := cmd.Flags().GetBool("session")

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	// Resolve agent/profile/model.
	prof, commandOverride, err := resolveAskProfile(cmd)
	if err != nil {
		return err
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	planFlag, _ := cmd.Flags().GetString("plan")
	effectivePlanID, err := resolveEffectivePlanID(s, projCfg, planFlag, cmd.Flags().Changed("plan"))
	if err != nil {
		return err
	}

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// Build the full prompt with project context if needed.
	fullPrompt, err := buildAskPrompt(s, projCfg, effectivePlanID, prompt)
	if err != nil {
		return err
	}

	// For count > 1, we run multiple sequential sessions.
	// For count == 1, this is just one session.
	var lastOutput string
	for i := 0; i < count; i++ {
		iterPrompt := fullPrompt
		if chain && i > 0 && lastOutput != "" {
			iterPrompt += "\n\n## Previous Run Output\n\n" + lastOutput + "\n"
		}

		loopDef, maxCycles := buildAskLoopDefinition(prof.Name, iterPrompt)

		var commandOverrides map[string]string
		if commandOverride != "" {
			commandOverrides = map[string]string{
				prof.Agent: commandOverride,
			}
		}

		dcfg := session.DaemonConfig{
			ProjectDir:            workDir,
			ProjectName:           projCfg.Name,
			WorkDir:               workDir,
			PlanID:                effectivePlanID,
			ProfileName:           prof.Name,
			AgentName:             prof.Agent,
			Loop:                  loopDef,
			Profiles:              []config.Profile{*prof},
			MaxCycles:             maxCycles,
			AgentCommandOverrides: commandOverrides,
		}

		sessionID, err := session.CreateSession(dcfg)
		if err != nil {
			return fmt.Errorf("creating session: %w", err)
		}
		debug.LogKV("cli.ask", "session created",
			"session_id", sessionID,
			"agent", prof.Agent,
			"iteration", i+1,
			"count", count,
		)

		if err := session.StartDaemon(sessionID); err != nil {
			debug.LogKV("cli.ask", "daemon start failed", "session_id", sessionID, "error", err)
			return fmt.Errorf("starting session daemon: %w", err)
		}

		if sessionMode {
			fmt.Printf("\n  %sSession #%d started%s (agent=%s, project=%s)\n",
				styleBoldGreen, sessionID, colorReset, prof.Agent, projCfg.Name)
			if count > 1 {
				fmt.Printf("  Run %d of %d\n", i+1, count)
			}
			fmt.Printf("  Use %sadaf attach %d%s to connect.\n\n", styleBoldWhite, sessionID, colorReset)
			continue
		}

		output, exitCode, err := streamAskSession(cmd.Context(), sessionID, projCfg.Name, prof.Agent, effectivePlanID, i+1, count)
		if err != nil {
			return err
		}
		lastOutput = output

		// If the agent exited with a non-zero code on the last iteration, propagate it.
		if i == count-1 && exitCode != 0 {
			os.Exit(exitCode)
		}
	}

	if sessionMode && count > 1 {
		fmt.Printf("  %sAll %d sessions started.%s\n", styleBoldGreen, count, colorReset)
		fmt.Printf("  Use %sadaf sessions%s to list them.\n\n", styleBoldWhite, colorReset)
	}

	return nil
}

// resolveAskPrompt extracts the prompt from positional args, --prompt flag, or stdin.
func resolveAskPrompt(cmd *cobra.Command, args []string) (string, error) {
	promptFlag, _ := cmd.Flags().GetString("prompt")
	promptFlag = strings.TrimSpace(promptFlag)

	// Positional argument takes precedence.
	if len(args) > 0 {
		joined := strings.TrimSpace(strings.Join(args, " "))
		if joined != "" {
			return joined, nil
		}
	}

	// --prompt flag.
	if promptFlag != "" {
		return promptFlag, nil
	}

	// Try stdin (only if not a terminal).
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading prompt from stdin: %w", err)
		}
		if s := strings.TrimSpace(string(data)); s != "" {
			return s, nil
		}
	}

	// No prompt provided — standalone mode.
	fmt.Println("  No prompt provided — running in standalone mode with project context.")
	return "", nil
}

// resolveAskProfile resolves the agent profile from --profile, --agent/--model flags.
func resolveAskProfile(cmd *cobra.Command) (*config.Profile, string, error) {
	customCmd, _ := cmd.Flags().GetString("command")
	reasoningLevel, _ := cmd.Flags().GetString("reasoning-level")
	prof, _, cmdOverride, err := resolveProfile(cmd, ProfileResolveOpts{
		Prefix:         "ask",
		CustomCmd:      strings.TrimSpace(customCmd),
		ReasoningLevel: strings.TrimSpace(reasoningLevel),
	})
	return prof, cmdOverride, err
}

// buildAskPrompt wraps the user prompt with project context.
// When userPrompt is empty (standalone mode), returns just the project context.
func buildAskPrompt(s *store.Store, projCfg *store.ProjectConfig, planID, userPrompt string) (string, error) {
	built, err := promptpkg.Build(promptpkg.BuildOpts{
		Store:   s,
		Project: projCfg,
		PlanID:  planID,
	})
	if err != nil {
		debug.LogKV("cli.ask", "prompt.Build failed, using raw prompt", "error", err)
		if userPrompt == "" {
			return "Work on the project using the available context.", nil
		}
		return userPrompt, nil
	}

	// Standalone mode: return just the project context without a task section.
	if userPrompt == "" {
		return built, nil
	}

	// The built prompt includes project context (rules, context, etc.).
	// Append the user's specific task as an objective.
	return built + "\n\n# Task\n\n" + userPrompt + "\n", nil
}

// buildAskLoopDefinition creates a minimal 1-step loop for standalone execution.
func buildAskLoopDefinition(profileName, prompt string) (config.LoopDef, int) {
	return config.LoopDef{
		Name: "ask",
		Steps: []config.LoopStep{
			{
				Profile:      profileName,
				Turns:        1,
				Instructions: prompt,
			},
		},
	}, 1 // maxCycles = 1 (single run)
}

// streamAskSession connects to a daemon session, streams output, and returns
// the agent's captured output and exit code.
func streamAskSession(parentCtx context.Context, sessionID int, projectName, agentName, planID string, iteration, total int) (string, int, error) {
	client, err := session.ConnectToSession(sessionID)
	if err != nil {
		session.AbortSessionStartup(sessionID, "cli attach failed: "+err.Error())
		return "", 1, fmt.Errorf("connecting to session %d: %w", sessionID, err)
	}
	defer client.Close()

	fmt.Println()
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	if total > 1 {
		fmt.Printf(styleBoldCyan+"   adaf ask [%d/%d]"+colorReset+"\n", iteration, total)
	} else {
		fmt.Println(styleBoldCyan + "   adaf ask" + colorReset)
	}
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println()
	printField("Project", projectName)
	printField("Agent", agentName)
	if planID != "" {
		printField("Plan", planID)
	}
	printField("Session", fmt.Sprintf("#%d", sessionID))
	printField("Started", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()
	fmt.Println(colorDim + "  " + strings.Repeat("-", 46) + colorReset)
	fmt.Println()

	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		fmt.Printf("\n  %sReceived interrupt, cancelling...%s\n", styleBoldYellow, colorReset)
		_ = client.Cancel()
		cancel()
	}()

	eventCh := make(chan any, 256)
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.StreamEvents(eventCh, nil)
	}()

	display := stream.NewDisplay(os.Stdout)
	defer display.Finish()

	var lastOutput strings.Builder
	exitCode := 0

	for {
		select {
		case <-ctx.Done():
			_ = client.Cancel()
		case msg, ok := <-eventCh:
			if !ok {
				if streamErr := <-errCh; streamErr != nil {
					return lastOutput.String(), 1, streamErr
				}
				fmt.Printf("\n  %sAsk completed.%s\n\n", styleBoldGreen, colorReset)
				return lastOutput.String(), exitCode, nil
			}
			switch ev := msg.(type) {
			case events.AgentRawOutputMsg:
				if ev.Data != "" {
					fmt.Print(ev.Data)
					lastOutput.WriteString(ev.Data)
				}
			case events.AgentEventMsg:
				display.Handle(ev.Event)
			case events.AgentFinishedMsg:
				if ev.Result != nil && ev.Result.ExitCode != 0 {
					exitCode = ev.Result.ExitCode
				}
			case events.LoopDoneMsg:
				if ev.Err != nil && ev.Reason != "cancelled" {
					return lastOutput.String(), 1, fmt.Errorf("agent error: %w", ev.Err)
				}
			}
		}
	}
}
