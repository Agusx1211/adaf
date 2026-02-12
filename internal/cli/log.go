package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var turnCmd = &cobra.Command{
	Use:     "turn",
	Aliases: []string{"turns", "log", "logs", "turn-log"},
	Short:   "Manage turn records",
	Long: `View and create turn records that track what each agent turn accomplished.

Turn records capture the objective, what was built, key decisions, challenges,
current state, known issues, and next steps. They serve as the handoff
mechanism between agent turns, enabling relay-style collaboration.

Examples:
  adaf turn list                                    # List all turns
  adaf turn latest                                  # Show most recent
  adaf turn show 5                                  # Show turn #5
  adaf turn create --agent claude --objective "Fix auth" --built "JWT implementation"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var turnListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all turns",
	RunE:    runTurnList,
}

var turnShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show a full turn record",
	Args:    cobra.ExactArgs(1),
	RunE:    runTurnShow,
}

var turnLatestCmd = &cobra.Command{
	Use:     "latest",
	Aliases: []string{"last", "recent"},
	Short:   "Show the most recent turn record",
	RunE:    runTurnLatest,
}

var turnCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add"},
	Short:   "Create a new turn record entry",
	RunE:    runTurnCreate,
}

func init() {
	turnListCmd.Flags().String("plan", "", "Filter turns by plan ID")

	turnCreateCmd.Flags().String("agent", "", "Agent name (required)")
	turnCreateCmd.Flags().String("model", "", "Agent model")
	turnCreateCmd.Flags().String("commit", "", "Commit hash")
	turnCreateCmd.Flags().String("plan", "", "Plan ID associated with this turn")
	turnCreateCmd.Flags().String("objective", "", "Turn objective (required)")
	turnCreateCmd.Flags().String("built", "", "What was built")
	turnCreateCmd.Flags().String("decisions", "", "Key decisions made")
	turnCreateCmd.Flags().String("challenges", "", "Challenges encountered")
	turnCreateCmd.Flags().String("state", "", "Current state of the project")
	turnCreateCmd.Flags().String("issues", "", "Known issues")
	turnCreateCmd.Flags().String("next", "", "Next steps")
	turnCreateCmd.Flags().String("build-state", "", "Build state (compiles, tests pass, etc.)")
	turnCreateCmd.Flags().Int("duration", 0, "Turn duration in seconds")
	_ = turnCreateCmd.MarkFlagRequired("agent")
	_ = turnCreateCmd.MarkFlagRequired("objective")

	turnCmd.AddCommand(turnListCmd)
	turnCmd.AddCommand(turnShowCmd)
	turnCmd.AddCommand(turnLatestCmd)
	turnCmd.AddCommand(turnCreateCmd)
	rootCmd.AddCommand(turnCmd)
}

func runTurnList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	planFilter, _ := cmd.Flags().GetString("plan")
	planFilter = strings.TrimSpace(planFilter)
	if planFilter != "" {
		if err := validatePlanID(planFilter); err != nil {
			return err
		}
	}

	turns, err := s.ListTurns()
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}
	if planFilter != "" {
		var filtered []store.Turn
		for _, t := range turns {
			if t.PlanID == planFilter {
				filtered = append(filtered, t)
			}
		}
		turns = filtered
	}

	printHeader("Turns")

	if len(turns) == 0 {
		fmt.Printf("  %sNo turns found.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"ID", "HEX", "DATE", "AGENT", "PLAN", "OBJECTIVE", "BUILD"}
	var rows [][]string
	for _, t := range turns {
		plan := "shared"
		if t.PlanID != "" {
			plan = t.PlanID
		}
		hexCol := ""
		if t.HexID != "" {
			hexCol = t.HexID
		}
		rows = append(rows, []string{
			fmt.Sprintf("#%d", t.ID),
			hexCol,
			t.Date.Format("2006-01-02 15:04"),
			t.Agent,
			plan,
			truncate(t.Objective, 40),
			truncate(t.BuildState, 15),
		})
	}
	printTable(headers, rows)

	fmt.Printf("\n  %sTotal: %d turn(s)%s\n\n", colorDim, len(turns), colorReset)
	return nil
}

func runTurnShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	// Try integer ID first, fall back to hex ID lookup.
	id, err := strconv.Atoi(args[0])
	if err == nil {
		turn, err := s.GetTurn(id)
		if err != nil {
			return fmt.Errorf("getting turn #%d: %w", id, err)
		}
		printTurn(turn)
		return nil
	}

	// Hex ID lookup.
	hexID := strings.TrimSpace(args[0])
	turns, err := s.ListTurns()
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}
	for i := range turns {
		if turns[i].HexID == hexID {
			printTurn(&turns[i])
			return nil
		}
	}
	return fmt.Errorf("turn not found for ID %q", args[0])
}

func runTurnLatest(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	turn, err := s.LatestTurn()
	if err != nil {
		return fmt.Errorf("getting latest turn: %w", err)
	}

	if turn == nil {
		fmt.Println()
		fmt.Printf("  %sNo turns found.%s\n\n", colorDim, colorReset)
		return nil
	}

	printTurn(turn)
	return nil
}

func runTurnCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	agent, _ := cmd.Flags().GetString("agent")
	model, _ := cmd.Flags().GetString("model")
	commit, _ := cmd.Flags().GetString("commit")
	planID, _ := cmd.Flags().GetString("plan")
	planID = strings.TrimSpace(planID)
	if planID != "" {
		if err := validatePlanID(planID); err != nil {
			return err
		}
		plan, err := s.GetPlan(planID)
		if err != nil {
			return fmt.Errorf("loading plan %q: %w", planID, err)
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", planID)
		}
	}
	objective, _ := cmd.Flags().GetString("objective")
	built, _ := cmd.Flags().GetString("built")
	decisions, _ := cmd.Flags().GetString("decisions")
	challenges, _ := cmd.Flags().GetString("challenges")
	state, _ := cmd.Flags().GetString("state")
	issues, _ := cmd.Flags().GetString("issues")
	next, _ := cmd.Flags().GetString("next")
	buildState, _ := cmd.Flags().GetString("build-state")
	duration, _ := cmd.Flags().GetInt("duration")

	turn := &store.Turn{
		Agent:        agent,
		AgentModel:   model,
		CommitHash:   commit,
		PlanID:       planID,
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

	if err := s.CreateTurn(turn); err != nil {
		return fmt.Errorf("creating turn: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sTurn #%d created.%s\n", styleBoldGreen, turn.ID, colorReset)
	printField("Agent", turn.Agent)
	if turn.PlanID != "" {
		printField("Plan", turn.PlanID)
	}
	printField("Objective", turn.Objective)
	fmt.Println()

	return nil
}

func printTurn(turn *store.Turn) {
	turnLabel := fmt.Sprintf("Turn #%d", turn.ID)
	if turn.HexID != "" {
		turnLabel += fmt.Sprintf(" [%s]", turn.HexID)
	}
	printHeader(turnLabel)

	printField("Date", turn.Date.Format("2006-01-02 15:04:05 UTC"))
	printField("Agent", turn.Agent)
	if turn.HexID != "" {
		printField("Hex ID", turn.HexID)
	}
	if turn.LoopRunHexID != "" {
		printField("Loop Run Hex ID", turn.LoopRunHexID)
	}
	if turn.StepHexID != "" {
		printField("Step Hex ID", turn.StepHexID)
	}
	if turn.AgentModel != "" {
		printField("Model", turn.AgentModel)
	}
	if turn.CommitHash != "" {
		printField("Commit", turn.CommitHash)
	}
	if turn.DurationSecs > 0 {
		mins := turn.DurationSecs / 60
		secs := turn.DurationSecs % 60
		printField("Duration", fmt.Sprintf("%dm %ds", mins, secs))
	}
	if turn.BuildState != "" {
		printField("Build State", turn.BuildState)
	}
	if turn.PlanID != "" {
		printField("Plan", turn.PlanID)
	}

	printLogSection("Objective", turn.Objective)
	printLogSection("What Was Built", turn.WhatWasBuilt)
	printLogSection("Key Decisions", turn.KeyDecisions)
	printLogSection("Challenges", turn.Challenges)
	printLogSection("Current State", turn.CurrentState)
	printLogSection("Known Issues", turn.KnownIssues)
	printLogSection("Next Steps", turn.NextSteps)

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
