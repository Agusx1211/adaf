package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/looprun"
	"github.com/agusx1211/adaf/internal/pushover"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

var loopCmd = &cobra.Command{
	Use:     "loop",
	Aliases: []string{"loops"},
	Short:   "Manage and run loops",
	Long: `Loops are cyclic templates of profile steps. Each step runs an agent profile
for N turns, with support for inter-step messaging and stop signals.

Loops are defined in ~/.adaf/config.json and can chain multiple agent profiles
together (e.g., a "junior" agent writes code, then a "senior" agent reviews).
Steps can send Pushover notifications, post messages to subsequent steps,
and signal the loop to stop.

Examples:
  adaf loop list                          # Show defined loops
  adaf loop start dev-cycle               # Start a loop
  adaf loop status                        # Check active loop
  adaf loop stop                          # Signal loop to stop
  adaf loop message "auth module done"    # Post inter-step message
  adaf loop notify "Done" "Build passed"  # Send push notification`,
}

var loopListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List loop definitions from config",
	RunE:    loopList,
}

var loopStartCmd = &cobra.Command{
	Use:     "start <name>",
	Aliases: []string{"run", "begin"},
	Short:   "Start a loop session",
	Args:    cobra.ExactArgs(1),
	RunE:    loopStart,
}

var loopStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"halt", "cancel", "end"},
	Short:   "Signal the current loop to stop",
	Long:    "Reads ADAF_LOOP_RUN_ID from environment and signals the loop to stop after the current step.",
	RunE:    loopStop,
}

var loopMessageCmd = &cobra.Command{
	Use:     "message <text>",
	Aliases: []string{"msg", "send"},
	Short:   "Post a message to subsequent loop steps",
	Long:    "Reads ADAF_LOOP_RUN_ID and ADAF_LOOP_STEP_INDEX from environment.",
	Args:    cobra.ExactArgs(1),
	RunE:    loopMessage,
}

var loopNotifyCmd = &cobra.Command{
	Use:   "notify <title> <message>",
	Short: "Send a Pushover notification",
	Long: `Send a Pushover notification from a loop step.
Reads ADAF_LOOP_RUN_ID from environment to confirm running inside a loop.

Title: max 250 characters.
Message: max 1024 characters.

Use --priority to set notification priority:
  -2 = lowest, -1 = low, 0 = normal (default), 1 = high`,
	Args: cobra.ExactArgs(2),
	RunE: loopNotify,
}

var loopStatusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"info", "state"},
	Short:   "Show the active loop run status",
	RunE:    loopStatus,
}

func init() {
	loopStartCmd.Flags().String("plan", "", "Plan ID override for this loop run (defaults to active plan)")
	loopStartCmd.Flags().Bool("foreground", false, "Run inline in the current process (do not fork daemon)")
	loopNotifyCmd.Flags().IntP("priority", "p", 0, "Notification priority (-2 to 1)")
	loopCmd.AddCommand(loopListCmd, loopStartCmd, loopStopCmd, loopMessageCmd, loopNotifyCmd, loopStatusCmd)
	rootCmd.AddCommand(loopCmd)
}

func loopList(cmd *cobra.Command, args []string) error {
	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(globalCfg.Loops) == 0 {
		fmt.Println(colorDim + "  No loops defined. Add loops to ~/.adaf/config.json." + colorReset)
		return nil
	}

	printHeader("Loops")
	for _, l := range globalCfg.Loops {
		fmt.Printf("  %s%s%s\n", styleBoldCyan, l.Name, colorReset)
		for i, step := range l.Steps {
			turns := step.Turns
			if turns <= 0 {
				turns = 1
			}
			role := config.EffectiveStepRole(step.Role)
			spawnCount := 0
			if step.Delegation != nil {
				spawnCount = len(step.Delegation.Profiles)
			}
			flags := ""
			if step.CanStop {
				flags += " [can_stop]"
			}
			if step.CanMessage {
				flags += " [can_message]"
			}
			if step.CanPushover {
				flags += " [can_pushover]"
			}
			spawnTag := " [no-spawn]"
			if spawnCount > 0 {
				spawnTag = fmt.Sprintf(" [spawn:%d]", spawnCount)
			}
			fmt.Printf("    %d. %s (%s) x%d%s%s\n", i+1, step.Profile, role, turns, spawnTag, flags)
			if step.Instructions != "" {
				instr := step.Instructions
				if len(instr) > 60 {
					instr = instr[:57] + "..."
				}
				fmt.Printf("       %s%s%s\n", colorDim, instr, colorReset)
			}
		}
		fmt.Println()
	}
	return nil
}

func loopStart(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("loop start is not available inside an agent context")
	}

	loopName := args[0]
	planFlag, _ := cmd.Flags().GetString("plan")
	foreground, _ := cmd.Flags().GetBool("foreground")

	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	if err := s.EnsureDirs(); err != nil {
		return err
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	loopDef := globalCfg.FindLoop(loopName)
	if loopDef == nil {
		return fmt.Errorf("loop %q not found", loopName)
	}

	if len(loopDef.Steps) == 0 {
		return fmt.Errorf("loop %q has no steps", loopName)
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}
	effectivePlanID, err := resolveEffectivePlanID(s, projCfg, planFlag, cmd.Flags().Changed("plan"))
	if err != nil {
		return err
	}

	profiles, err := loopProfilesSnapshot(globalCfg, loopDef)
	if err != nil {
		return err
	}

	if foreground {
		return runLoopForeground(cmd.Context(), s, globalCfg, loopDef, projCfg, effectivePlanID)
	}

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	dcfg := session.DaemonConfig{
		ProjectDir:  workDir,
		ProjectName: projCfg.Name,
		WorkDir:     workDir,
		PlanID:      effectivePlanID,
		ProfileName: loopDef.Name,
		AgentName:   "loop",
		Loop:        *loopDef,
		Profiles:    profiles,
		Pushover:    globalCfg.Pushover,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		return fmt.Errorf("creating loop session: %w", err)
	}
	if err := session.StartDaemon(sessionID); err != nil {
		session.AbortSessionStartup(sessionID, "loop start daemon failed: "+err.Error())
		return fmt.Errorf("starting loop daemon: %w", err)
	}

	fmt.Printf("\n  %sLoop session #%d started%s (loop=%s, project=%s)\n",
		styleBoldGreen, sessionID, colorReset, loopDef.Name, projCfg.Name)
	if effectivePlanID != "" {
		fmt.Printf("  Plan: %s%s%s\n", styleBoldWhite, effectivePlanID, colorReset)
	}

	if isatty.IsTerminal(os.Stdout.Fd()) {
		return runAttach(cmd, []string{strconv.Itoa(sessionID)})
	}

	fmt.Printf("  Use %sadaf attach %d%s to connect.\n", styleBoldWhite, sessionID, colorReset)
	fmt.Printf("  Use %sadaf sessions%s to list all sessions.\n\n", styleBoldWhite, colorReset)
	return nil
}

func runLoopForeground(parentCtx context.Context, s *store.Store, globalCfg *config.GlobalConfig, loopDef *config.LoopDef, projCfg *store.ProjectConfig, planID string) error {
	agentsCfg, err := agent.LoadAgentsConfig()
	if err != nil {
		return fmt.Errorf("loading agent configuration: %w", err)
	}
	agent.PopulateFromConfig(agentsCfg)

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	fmt.Println()
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println(styleBoldCyan + "   adaf loop (foreground) — " + loopDef.Name + colorReset)
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println()
	printField("Project", projCfg.Name)
	if planID != "" {
		printField("Plan", planID)
	}
	printField("Loop", loopDef.Name)
	printField("Steps", fmt.Sprintf("%d", len(loopDef.Steps)))
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
		fmt.Printf("\n  %sReceived interrupt, cancelling loop...%s\n", styleBoldYellow, colorReset)
		cancel()
	}()

	eventCh := make(chan any, 256)
	runErrCh := make(chan error, 1)
	go func() {
		runErr := looprun.Run(ctx, looprun.RunConfig{
			Store:     s,
			GlobalCfg: globalCfg,
			LoopDef:   loopDef,
			Project:   projCfg,
			AgentsCfg: agentsCfg,
			PlanID:    planID,
			WorkDir:   workDir,
		}, eventCh)
		runErrCh <- runErr
		close(eventCh)
	}()

	display := stream.NewDisplay(os.Stdout)
	defer display.Finish()

	for msg := range eventCh {
		switch ev := msg.(type) {
		case runtui.AgentRawOutputMsg:
			if ev.Data != "" {
				fmt.Print(ev.Data)
			}
		case runtui.AgentEventMsg:
			display.Handle(ev.Event)
		case runtui.LoopStepStartMsg:
			totalSteps := ev.TotalSteps
			if totalSteps <= 0 {
				totalSteps = ev.StepIndex + 1
			}
			hexInfo := ""
			if ev.RunHexID != "" || ev.StepHexID != "" {
				hexInfo = " ["
				if ev.RunHexID != "" {
					hexInfo += "run:" + ev.RunHexID
				}
				if ev.StepHexID != "" {
					if ev.RunHexID != "" {
						hexInfo += " "
					}
					hexInfo += "step:" + ev.StepHexID
				}
				hexInfo += "]"
			}
			fmt.Printf("  %s[loop]%s cycle=%d step=%d/%d profile=%s turns=%d%s\n",
				colorDim, colorReset, ev.Cycle+1, ev.StepIndex+1, totalSteps, ev.Profile, ev.Turns, hexInfo)
		case runtui.LoopStepEndMsg:
			totalSteps := ev.TotalSteps
			if totalSteps <= 0 {
				totalSteps = ev.StepIndex + 1
			}
			stepHexTag := ""
			if ev.StepHexID != "" {
				stepHexTag = " [step:" + ev.StepHexID + "]"
			}
			fmt.Printf("  %s[loop]%s step=%d/%d profile=%s completed%s\n",
				colorDim, colorReset, ev.StepIndex+1, totalSteps, ev.Profile, stepHexTag)
		case runtui.AgentStartedMsg:
			hexTag := ""
			if ev.TurnHexID != "" {
				hexTag = " [" + ev.TurnHexID + "]"
			}
			fmt.Printf("  %s>>> Turn #%d%s starting%s\n", styleBoldGreen, ev.SessionID, hexTag, colorReset)
		case runtui.AgentFinishedMsg:
			if ev.Result != nil {
				hexTag := ""
				if ev.TurnHexID != "" {
					hexTag = " [" + ev.TurnHexID + "]"
				}
				fmt.Printf("  %s<<< Turn #%d%s completed (exit=%d, %s)%s\n",
					styleBoldGreen, ev.SessionID, hexTag, ev.Result.ExitCode, ev.Result.Duration.Round(time.Second), colorReset)
			}
		}
	}

	runErr := <-runErrCh
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return fmt.Errorf("loop run failed: %w", runErr)
	}

	reason := "stopped"
	if errors.Is(runErr, context.Canceled) {
		reason = "cancelled"
	}
	fmt.Printf("\n  %sLoop finished (%s).%s\n\n", styleBoldGreen, reason, colorReset)
	return nil
}

func loopProfilesSnapshot(globalCfg *config.GlobalConfig, loopDef *config.LoopDef) ([]config.Profile, error) {
	seen := make(map[string]struct{}, len(loopDef.Steps))
	profiles := make([]config.Profile, 0, len(loopDef.Steps))

	addProfile := func(name string) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			return nil
		}
		prof := globalCfg.FindProfile(name)
		if prof == nil {
			return fmt.Errorf("profile %q not found for loop %q", name, loopDef.Name)
		}
		seen[key] = struct{}{}
		profiles = append(profiles, *prof)
		return nil
	}

	for _, step := range loopDef.Steps {
		if strings.TrimSpace(step.Profile) == "" {
			return nil, fmt.Errorf("loop %q has a step with empty profile", loopDef.Name)
		}
		if err := addProfile(step.Profile); err != nil {
			return nil, err
		}
		// Include all profiles from the full delegation tree so the daemon has
		// everything needed for nested spawn resolution and prompt rendering.
		for _, name := range config.CollectDelegationProfileNames(step.Delegation) {
			if err := addProfile(name); err != nil {
				return nil, err
			}
		}
	}
	return profiles, nil
}

func loopStop(cmd *cobra.Command, args []string) error {
	runIDStr := os.Getenv("ADAF_LOOP_RUN_ID")
	if runIDStr == "" {
		return fmt.Errorf("ADAF_LOOP_RUN_ID not set (are you running inside a loop step?)")
	}
	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		return fmt.Errorf("invalid ADAF_LOOP_RUN_ID: %s", runIDStr)
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	if err := s.SignalLoopStop(runID); err != nil {
		return fmt.Errorf("signaling stop: %w", err)
	}

	fmt.Printf("  %sLoop stop signal sent for run #%d.%s\n", styleBoldGreen, runID, colorReset)
	return nil
}

func loopMessage(cmd *cobra.Command, args []string) error {
	runIDStr := os.Getenv("ADAF_LOOP_RUN_ID")
	stepIdxStr := os.Getenv("ADAF_LOOP_STEP_INDEX")
	if runIDStr == "" {
		return fmt.Errorf("ADAF_LOOP_RUN_ID not set (are you running inside a loop step?)")
	}
	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		return fmt.Errorf("invalid ADAF_LOOP_RUN_ID: %s", runIDStr)
	}
	stepIdx, _ := strconv.Atoi(stepIdxStr) // 0 if not set

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	msg := &store.LoopMessage{
		RunID:     runID,
		StepIndex: stepIdx,
		Content:   args[0],
	}
	if err := s.CreateLoopMessage(msg); err != nil {
		return fmt.Errorf("creating message: %w", err)
	}

	fmt.Printf("  %sMessage posted for loop run #%d.%s\n", styleBoldGreen, runID, colorReset)
	return nil
}

func loopStatus(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	run, err := s.ActiveLoopRun()
	if err != nil {
		return fmt.Errorf("loading active loop run: %w", err)
	}
	if run == nil {
		fmt.Println(colorDim + "  No active loop run." + colorReset)
		return nil
	}

	printHeader("Active Loop Run")
	runIDLabel := fmt.Sprintf("#%d", run.ID)
	if run.HexID != "" {
		runIDLabel = fmt.Sprintf("#%d [%s]", run.ID, run.HexID)
	}
	printField("Run ID", runIDLabel)
	printField("Loop", run.LoopName)
	printField("Status", run.Status)
	printField("Cycle", fmt.Sprintf("%d", run.Cycle+1))
	if run.StepIndex < len(run.Steps) {
		step := run.Steps[run.StepIndex]
		stepLabel := fmt.Sprintf("%d/%d (%s)", run.StepIndex+1, len(run.Steps), step.Profile)
		stepKey := fmt.Sprintf("%d:%d", run.Cycle, run.StepIndex)
		if hexID, ok := run.StepHexIDs[stepKey]; ok {
			stepLabel += fmt.Sprintf(" [%s]", hexID)
		}
		printField("Current Step", stepLabel)
	}
	printField("Turns", fmt.Sprintf("%d", len(run.TurnIDs)))
	printField("Started", run.StartedAt.Format("2006-01-02 15:04:05"))
	if s.IsLoopStopped(run.ID) {
		printFieldColored("Stop Signal", "received", colorYellow)
	}

	// Show messages.
	msgs, _ := s.ListLoopMessages(run.ID)
	if len(msgs) > 0 {
		fmt.Println()
		printHeader("Messages")
		for _, msg := range msgs {
			fmt.Printf("  [step %d, %s] %s\n", msg.StepIndex+1, msg.CreatedAt.Format("15:04:05"), msg.Content)
		}
	}

	return nil
}

func loopNotify(cmd *cobra.Command, args []string) error {
	runIDStr := os.Getenv("ADAF_LOOP_RUN_ID")
	if runIDStr == "" {
		return fmt.Errorf("ADAF_LOOP_RUN_ID not set (are you running inside a loop step?)")
	}

	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !pushover.Configured(&globalCfg.Pushover) {
		return fmt.Errorf("pushover not configured: run 'adaf config pushover setup' to set credentials")
	}

	priority, _ := cmd.Flags().GetInt("priority")
	if priority < -2 || priority > 1 {
		return fmt.Errorf("priority must be between -2 and 1")
	}

	msg := pushover.Message{
		Title:    args[0],
		Body:     args[1],
		Priority: priority,
	}

	if err := pushover.Send(&globalCfg.Pushover, msg); err != nil {
		return fmt.Errorf("sending notification: %w", err)
	}

	fmt.Printf("  %sPushover notification sent.%s\n", styleBoldGreen, colorReset)
	return nil
}
