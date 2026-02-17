package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/events"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

var runCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"execute", "exec"},
	Short:   "Run an agent loop against the project (daemon-backed)",
	Long: `Run an AI agent against the project in a continuous loop. Execution is
daemon-backed and follows the loop runtime model used throughout ADAF.

For a single-turn standalone session, use 'adaf ask' instead — it runs the
agent once with full project context and exits.

Supported agents: claude, codex, vibe, opencode, gemini, generic

Examples:
  adaf run --agent claude --max-turns 1
  adaf run --agent codex --model gpt-5.1-codex-max
  adaf run --agent claude --prompt "Fix the failing tests in auth/"
  adaf run --agent claude -s`,
	RunE: runAgent,
}

func init() {
	runCmd.Flags().String("agent", "claude", "Agent to run (claude, codex, vibe, opencode, generic)")
	runCmd.Flags().String("prompt", "", "Prompt/instructions for the run (default: built from project context)")
	runCmd.Flags().String("plan", "", "Plan ID override for this run (defaults to active plan)")
	runCmd.Flags().Int("max-turns", 0, "Maximum turns for this run (0 = unlimited)")
	runCmd.Flags().String("model", "", "Model override for the agent")
	runCmd.Flags().String("command", "", "Custom command path for the selected agent")
	runCmd.Flags().String("reasoning-level", "", "Reasoning level (e.g. low, medium, high, xhigh)")
	runCmd.Flags().BoolP("session", "s", false, "Start and leave detached (use 'adaf attach' to connect)")
	runCmd.Flags().StringSlice("skills", nil, "Skill IDs to activate (e.g. autonomy,code_writing,commit)")
	rootCmd.AddCommand(runCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	debug.Log("cli.run", "runAgent() called")
	if session.IsAgentContext() {
		return fmt.Errorf("run is not available inside an agent context")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	if err := s.EnsureDirs(); err != nil {
		return err
	}

	agentName, _ := cmd.Flags().GetString("agent")
	prompt, _ := cmd.Flags().GetString("prompt")
	planFlag, _ := cmd.Flags().GetString("plan")
	maxTurns, _ := cmd.Flags().GetInt("max-turns")
	modelFlag, _ := cmd.Flags().GetString("model")
	customCmd, _ := cmd.Flags().GetString("command")
	reasoningLevel, _ := cmd.Flags().GetString("reasoning-level")
	sessionMode, _ := cmd.Flags().GetBool("session")
	skills, _ := cmd.Flags().GetStringSlice("skills")

	modelFlag = strings.TrimSpace(modelFlag)
	reasoningLevel = strings.TrimSpace(reasoningLevel)
	customCmd = strings.TrimSpace(customCmd)

	if _, ok := agent.Get(agentName); !ok {
		return fmt.Errorf("unknown agent %q (valid: %s)", agentName, strings.Join(agentNames(), ", "))
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading global config: %w", err)
	}

	agentsCfg, err := agent.LoadAgentsConfig()
	if err != nil {
		return fmt.Errorf("loading agent configuration: %w", err)
	}
	if rec, ok := agentsCfg.Agents[agentName]; ok && customCmd == "" && strings.TrimSpace(rec.Path) != "" {
		customCmd = strings.TrimSpace(rec.Path)
	}

	modelOverride := agent.ResolveModelOverride(agentsCfg, globalCfg, agentName)
	if modelFlag != "" {
		modelOverride = modelFlag
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	effectivePlanID, err := resolveEffectivePlanID(s, projCfg, planFlag, cmd.Flags().Changed("plan"))
	if err != nil {
		return err
	}

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	if prompt == "" {
		built, err := promptpkg.Build(promptpkg.BuildOpts{
			Store:   s,
			Project: projCfg,
			PlanID:  effectivePlanID,
		})
		if err != nil {
			return fmt.Errorf("building default prompt: %w", err)
		}
		prompt = built
	}

	profileName := fmt.Sprintf("run:%s", agentName)
	runProfile := config.Profile{
		Name:           profileName,
		Agent:          agentName,
		Model:          modelOverride,
		ReasoningLevel: reasoningLevel,
	}

	loopDef, maxCycles := buildRunLoopDefinition(agentName, profileName, prompt, maxTurns, globalCfg, skills)

	var commandOverrides map[string]string
	if customCmd != "" {
		commandOverrides = map[string]string{
			agentName: customCmd,
		}
	}

	dcfg := session.DaemonConfig{
		ProjectDir:            workDir,
		ProjectName:           projCfg.Name,
		WorkDir:               workDir,
		PlanID:                effectivePlanID,
		ProfileName:           profileName,
		AgentName:             agentName,
		Loop:                  loopDef,
		Profiles:              []config.Profile{runProfile},
		Pushover:              globalCfg.Pushover,
		MaxCycles:             maxCycles,
		AgentCommandOverrides: commandOverrides,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	debug.LogKV("cli.run", "session created",
		"session_id", sessionID,
		"agent", agentName,
		"plan_id", effectivePlanID,
		"max_turns", maxTurns,
		"session_mode", sessionMode,
	)
	if err := session.StartDaemon(sessionID); err != nil {
		debug.LogKV("cli.run", "daemon start failed", "session_id", sessionID, "error", err)
		return fmt.Errorf("starting session daemon: %w", err)
	}
	debug.LogKV("cli.run", "daemon started", "session_id", sessionID)

	if sessionMode {
		fmt.Printf("\n  %sSession #%d started%s (agent=%s, project=%s)\n",
			styleBoldGreen, sessionID, colorReset, agentName, projCfg.Name)
		fmt.Printf("  Use %sadaf attach %d%s to connect.\n", styleBoldWhite, sessionID, colorReset)
		fmt.Printf("  Use %sadaf sessions%s to list all sessions.\n\n", styleBoldWhite, colorReset)
		return nil
	}

	return streamRunSession(cmd.Context(), sessionID, projCfg.Name, agentName, effectivePlanID)
}

func buildRunLoopDefinition(agentName, profileName, prompt string, maxTurns int, globalCfg *config.GlobalConfig, skills []string) (config.LoopDef, int) {
	stepTurns, maxCycles := runTurnConfig(maxTurns)
	return config.LoopDef{
		Name: "run:" + agentName,
		Steps: []config.LoopStep{
			{
				Profile:      profileName,
				Position:     config.PositionLead,
				Turns:        stepTurns,
				Instructions: prompt,
				Skills:       skills,
			},
		},
	}, maxCycles
}

func runTurnConfig(maxTurns int) (stepTurns int, maxCycles int) {
	stepTurns = 1
	maxCycles = 0
	if maxTurns > 0 {
		stepTurns = maxTurns
		maxCycles = 1
	}
	return stepTurns, maxCycles
}

func streamRunSession(parentCtx context.Context, sessionID int, projectName, agentName, planID string) error {
	client, err := session.ConnectToSession(sessionID)
	if err != nil {
		session.AbortSessionStartup(sessionID, "cli attach failed: "+err.Error())
		return fmt.Errorf("connecting to session %d: %w", sessionID, err)
	}
	defer client.Close()

	fmt.Println()
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println(styleBoldCyan + "   adaf agent run" + colorReset)
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
		fmt.Printf("\n  %sReceived interrupt, cancelling run...%s\n", styleBoldYellow, colorReset)
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

	for {
		select {
		case <-ctx.Done():
			_ = client.Cancel()
		case msg, ok := <-eventCh:
			if !ok {
				if streamErr := <-errCh; streamErr != nil {
					return streamErr
				}
				fmt.Printf("\n  %sAgent loop finished.%s\n\n", styleBoldGreen, colorReset)
				return nil
			}
			switch ev := msg.(type) {
			case events.AgentRawOutputMsg:
				if ev.Data != "" {
					fmt.Print(ev.Data)
				}
			case events.AgentEventMsg:
				display.Handle(ev.Event)
			case events.LoopDoneMsg:
				if ev.Err != nil && ev.Reason != "cancelled" {
					return fmt.Errorf("loop error: %w", ev.Err)
				}
			}
		}
	}
}

func resolveEffectivePlanID(s *store.Store, projCfg *store.ProjectConfig, planFlag string, explicit bool) (string, error) {
	planID := strings.TrimSpace(planFlag)
	if !explicit {
		planID = strings.TrimSpace(projCfg.ActivePlanID)
	}
	if planID == "" {
		return "", nil
	}

	plan, err := s.GetPlan(planID)
	if err != nil {
		return "", fmt.Errorf("loading plan %q: %w", planID, err)
	}
	if plan == nil {
		return "", fmt.Errorf("plan %q not found", planID)
	}
	if plan.Status != "" && plan.Status != "active" {
		if explicit {
			return "", fmt.Errorf("plan %q is %q; only active plans can be used", planID, plan.Status)
		}
		return "", nil
	}
	return planID, nil
}

func agentNames() []string {
	all := agent.All()
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	return names
}
