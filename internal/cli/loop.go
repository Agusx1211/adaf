package cli

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/looprun"
	"github.com/agusx1211/adaf/internal/pushover"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/store"
)

var loopCmd = &cobra.Command{
	Use:     "loop",
	Aliases: []string{"loops"},
	Short:   "Manage and run loops",
	Long:  "Loops are cyclic templates of profile steps. Each step runs an agent profile for N turns, with support for inter-step messaging and stop signals.",
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
	Short:   "Start a loop (inline output)",
	Args:    cobra.ExactArgs(1),
	RunE:    loopStart,
}

var loopStopCmd = &cobra.Command{
	Use:     "stop",
	Aliases: []string{"halt", "cancel", "end"},
	Short:   "Signal the current loop to stop",
	Long:  "Reads ADAF_LOOP_RUN_ID from environment and signals the loop to stop after the current step.",
	RunE:  loopStop,
}

var loopMessageCmd = &cobra.Command{
	Use:     "message <text>",
	Aliases: []string{"msg", "send"},
	Short:   "Post a message to subsequent loop steps",
	Long:  "Reads ADAF_LOOP_RUN_ID and ADAF_LOOP_STEP_INDEX from environment.",
	Args:  cobra.ExactArgs(1),
	RunE:  loopMessage,
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
			fmt.Printf("    %d. %s x%d%s\n", i+1, step.Profile, turns, flags)
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
	loopName := args[0]

	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	s.EnsureDirs()

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

	agentsCfg, _ := agent.LoadAgentsConfig(s.Root())
	agent.PopulateFromConfig(agentsCfg)

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// Print header.
	fmt.Println()
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println(styleBoldCyan + "   adaf loop — " + loopDef.Name + colorReset)
	fmt.Println(styleBoldCyan + "  ==============================================" + colorReset)
	fmt.Println()
	printField("Project", projCfg.Name)
	printField("Loop", loopDef.Name)
	printField("Steps", fmt.Sprintf("%d", len(loopDef.Steps)))
	for i, step := range loopDef.Steps {
		turns := step.Turns
		if turns <= 0 {
			turns = 1
		}
		printField(fmt.Sprintf("  Step %d", i+1), fmt.Sprintf("%s x%d", step.Profile, turns))
	}
	printField("Started", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()
	fmt.Println(colorDim + "  " + strings.Repeat("-", 46) + colorReset)
	fmt.Println()

	eventCh := make(chan any, 256)

	cfg := looprun.RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   projCfg,
		AgentsCfg: agentsCfg,
		WorkDir:   workDir,
	}

	// Run loop in a goroutine.
	loopCancel := looprun.StartLoopRun(cfg, eventCh)
	defer loopCancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Printf("\n  %sReceived interrupt, finishing current step...%s\n", styleBoldYellow, colorReset)
		loopCancel()
	}()

	// Consume events inline.
	for msg := range eventCh {
		switch ev := msg.(type) {
		case runtui.LoopStepStartMsg:
			fmt.Printf("  %s>>> Cycle %d, Step %d: %s (x%d)%s\n",
				styleBoldCyan, ev.Cycle+1, ev.StepIndex+1, ev.Profile, ev.Turns, colorReset)
		case runtui.LoopStepEndMsg:
			fmt.Printf("  %s<<< Step %d: %s completed%s\n",
				styleBoldGreen, ev.StepIndex+1, ev.Profile, colorReset)
		case runtui.AgentStartedMsg:
			fmt.Printf("  %s>>> Session #%d starting%s\n", styleBoldGreen, ev.SessionID, colorReset)
		case runtui.AgentFinishedMsg:
			if ev.Result != nil {
				fmt.Printf("  %s<<< Session #%d completed (exit=%d, %s)%s\n",
					styleBoldGreen, ev.SessionID, ev.Result.ExitCode, ev.Result.Duration.Round(time.Second), colorReset)
			}
		case runtui.LoopDoneMsg:
			if ev.Err != nil && ev.Reason != "cancelled" {
				fmt.Printf("\n  %sLoop error: %v%s\n", styleBoldRed, ev.Err, colorReset)
			} else {
				fmt.Printf("\n  %sLoop finished (%s).%s\n", styleBoldGreen, ev.Reason, colorReset)
			}
		}
	}

	fmt.Println()
	return nil
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
	printField("Run ID", fmt.Sprintf("#%d", run.ID))
	printField("Loop", run.LoopName)
	printField("Status", run.Status)
	printField("Cycle", fmt.Sprintf("%d", run.Cycle+1))
	if run.StepIndex < len(run.Steps) {
		step := run.Steps[run.StepIndex]
		printField("Current Step", fmt.Sprintf("%d/%d (%s)", run.StepIndex+1, len(run.Steps), step.Profile))
	}
	printField("Sessions", fmt.Sprintf("%d", len(run.SessionIDs)))
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
		return fmt.Errorf("pushover not configured: run 'adaf pushover setup' to set credentials")
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
