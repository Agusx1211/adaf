package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:     "log",
	Aliases: []string{"logs", "session-log", "session_log", "session-logs", "session_logs"},
	Short:   "Manage session logs",
	Long: `View and create session logs that track what each agent session accomplished.

Session logs capture the objective, what was built, key decisions, challenges,
current state, known issues, and next steps. They serve as the handoff
mechanism between agent sessions, enabling relay-style collaboration.

Examples:
  adaf log list                                    # List all logs
  adaf log latest                                  # Show most recent
  adaf log show 5                                  # Show log #5
  adaf log create --agent claude --objective "Fix auth" --built "JWT implementation"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var logListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all session logs",
	RunE:    runLogList,
}

var logShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show a full session log",
	Args:    cobra.ExactArgs(1),
	RunE:    runLogShow,
}

var logLatestCmd = &cobra.Command{
	Use:     "latest",
	Aliases: []string{"last", "recent"},
	Short:   "Show the most recent session log",
	RunE:    runLogLatest,
}

var logCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add"},
	Short:   "Create a new session log entry",
	RunE:    runLogCreate,
}

func init() {
	logCreateCmd.Flags().String("agent", "", "Agent name (required)")
	logCreateCmd.Flags().String("model", "", "Agent model")
	logCreateCmd.Flags().String("commit", "", "Commit hash")
	logCreateCmd.Flags().String("objective", "", "Session objective (required)")
	logCreateCmd.Flags().String("built", "", "What was built")
	logCreateCmd.Flags().String("decisions", "", "Key decisions made")
	logCreateCmd.Flags().String("challenges", "", "Challenges encountered")
	logCreateCmd.Flags().String("state", "", "Current state of the project")
	logCreateCmd.Flags().String("issues", "", "Known issues")
	logCreateCmd.Flags().String("next", "", "Next steps")
	logCreateCmd.Flags().String("build-state", "", "Build state (compiles, tests pass, etc.)")
	logCreateCmd.Flags().Int("duration", 0, "Session duration in seconds")
	_ = logCreateCmd.MarkFlagRequired("agent")
	_ = logCreateCmd.MarkFlagRequired("objective")

	logCmd.AddCommand(logListCmd)
	logCmd.AddCommand(logShowCmd)
	logCmd.AddCommand(logLatestCmd)
	logCmd.AddCommand(logCreateCmd)
	rootCmd.AddCommand(logCmd)
}

func runLogList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	logs, err := s.ListLogs()
	if err != nil {
		return fmt.Errorf("listing logs: %w", err)
	}

	printHeader("Session Logs")

	if len(logs) == 0 {
		fmt.Printf("  %sNo session logs found.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"ID", "DATE", "AGENT", "OBJECTIVE", "BUILD"}
	var rows [][]string
	for _, l := range logs {
		rows = append(rows, []string{
			fmt.Sprintf("#%d", l.ID),
			l.Date.Format("2006-01-02 15:04"),
			l.Agent,
			truncate(l.Objective, 45),
			truncate(l.BuildState, 15),
		})
	}
	printTable(headers, rows)

	fmt.Printf("\n  %sTotal: %d session(s)%s\n\n", colorDim, len(logs), colorReset)
	return nil
}

func runLogShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid log ID %q: must be a number", args[0])
	}

	log, err := s.GetLog(id)
	if err != nil {
		return fmt.Errorf("getting log #%d: %w", id, err)
	}

	printSessionLog(log)
	return nil
}

func runLogLatest(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	log, err := s.LatestLog()
	if err != nil {
		return fmt.Errorf("getting latest log: %w", err)
	}

	if log == nil {
		fmt.Println()
		fmt.Printf("  %sNo session logs found.%s\n\n", colorDim, colorReset)
		return nil
	}

	printSessionLog(log)
	return nil
}

func runLogCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	agent, _ := cmd.Flags().GetString("agent")
	model, _ := cmd.Flags().GetString("model")
	commit, _ := cmd.Flags().GetString("commit")
	objective, _ := cmd.Flags().GetString("objective")
	built, _ := cmd.Flags().GetString("built")
	decisions, _ := cmd.Flags().GetString("decisions")
	challenges, _ := cmd.Flags().GetString("challenges")
	state, _ := cmd.Flags().GetString("state")
	issues, _ := cmd.Flags().GetString("issues")
	next, _ := cmd.Flags().GetString("next")
	buildState, _ := cmd.Flags().GetString("build-state")
	duration, _ := cmd.Flags().GetInt("duration")

	log := &store.SessionLog{
		Agent:        agent,
		AgentModel:   model,
		CommitHash:   commit,
		Objective:    objective,
		WhatWasBuilt: built,
		KeyDecisions: decisions,
		Challenges:   challenges,
		CurrentState: state,
		KnownIssues:  issues,
		NextSteps:    next,
		BuildState:   buildState,
		DurationSecs: duration,
	}

	if err := s.CreateLog(log); err != nil {
		return fmt.Errorf("creating log: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sSession log #%d created.%s\n", styleBoldGreen, log.ID, colorReset)
	printField("Agent", log.Agent)
	printField("Objective", log.Objective)
	fmt.Println()

	return nil
}

func printSessionLog(log *store.SessionLog) {
	printHeader(fmt.Sprintf("Session Log #%d", log.ID))

	printField("Date", log.Date.Format("2006-01-02 15:04:05 UTC"))
	printField("Agent", log.Agent)
	if log.AgentModel != "" {
		printField("Model", log.AgentModel)
	}
	if log.CommitHash != "" {
		printField("Commit", log.CommitHash)
	}
	if log.DurationSecs > 0 {
		mins := log.DurationSecs / 60
		secs := log.DurationSecs % 60
		printField("Duration", fmt.Sprintf("%dm %ds", mins, secs))
	}
	if log.BuildState != "" {
		printField("Build State", log.BuildState)
	}

	printLogSection("Objective", log.Objective)
	printLogSection("What Was Built", log.WhatWasBuilt)
	printLogSection("Key Decisions", log.KeyDecisions)
	printLogSection("Challenges", log.Challenges)
	printLogSection("Current State", log.CurrentState)
	printLogSection("Known Issues", log.KnownIssues)
	printLogSection("Next Steps", log.NextSteps)

	fmt.Println()
}

func printLogSection(title, content string) {
	if content == "" {
		return
	}
	fmt.Println()
	fmt.Printf("  %s%s:%s\n", colorBold, title, colorReset)
	for _, line := range strings.Split(content, "\n") {
		fmt.Printf("    %s\n", line)
	}
}
