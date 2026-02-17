package cli

import (
	"fmt"
	"os"
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
  adaf turn create --agent claude --objective "Fix auth" --built "JWT implementation"
  adaf turn update --built "Added JWT auth" --next "Add refresh tokens"`,
	RunE: runTurnList,
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

var turnUpdateCmd = &cobra.Command{
	Use:     "update [id]",
	Aliases: []string{"edit", "set"},
	Short:   "Update an existing turn record entry",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runTurnUpdate,
}

var turnSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search turn records by keyword",
	Long: `Search turn records for a keyword across all text fields.

Searches Objective, WhatWasBuilt, KeyDecisions, NextSteps, Challenges,
KnownIssues, and CurrentState fields (case-insensitive substring match).

Examples:
  adaf log search --query "authentication"
  adaf log search --query "refactor" --agent claude --limit 5
  adaf log search --query "test" --plan my-plan`,
	RunE: runTurnSearch,
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

	turnUpdateCmd.Flags().String("objective", "", "Turn objective")
	turnUpdateCmd.Flags().String("built", "", "What was built")
	turnUpdateCmd.Flags().String("decisions", "", "Key decisions made")
	turnUpdateCmd.Flags().String("challenges", "", "Challenges encountered")
	turnUpdateCmd.Flags().String("state", "", "Current state of the project")
	turnUpdateCmd.Flags().String("issues", "", "Known issues")
	turnUpdateCmd.Flags().String("next", "", "Next steps")
	turnUpdateCmd.Flags().String("build-state", "", "Build state (compiles, tests pass, etc.)")
	turnUpdateCmd.Flags().Int("duration", 0, "Turn duration in seconds")

	turnSearchCmd.Flags().String("query", "", "Search query (required)")
	turnSearchCmd.Flags().String("plan", "", "Filter by plan ID")
	turnSearchCmd.Flags().String("agent", "", "Filter by agent name")
	turnSearchCmd.Flags().Int("limit", 10, "Maximum number of results")
	_ = turnSearchCmd.MarkFlagRequired("query")

	turnCmd.AddCommand(turnListCmd)
	turnCmd.AddCommand(turnShowCmd)
	turnCmd.AddCommand(turnLatestCmd)
	turnCmd.AddCommand(turnCreateCmd)
	turnCmd.AddCommand(turnUpdateCmd)
	turnCmd.AddCommand(turnSearchCmd)
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

	turn, err := findTurnByIdentifier(s, args[0])
	if err != nil {
		return err
	}
	printTurn(turn)
	return nil
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

func runTurnUpdate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	turn, err := resolveTurnToUpdate(s, args)
	if err != nil {
		return err
	}

	updated := 0
	setStringFlag := func(flag string, target *string) error {
		if !cmd.Flags().Changed(flag) {
			return nil
		}
		value, err := cmd.Flags().GetString(flag)
		if err != nil {
			return err
		}
		*target = value
		updated++
		return nil
	}

	if err := setStringFlag("objective", &turn.Objective); err != nil {
		return err
	}
	if err := setStringFlag("built", &turn.WhatWasBuilt); err != nil {
		return err
	}
	if err := setStringFlag("decisions", &turn.KeyDecisions); err != nil {
		return err
	}
	if err := setStringFlag("challenges", &turn.Challenges); err != nil {
		return err
	}
	if err := setStringFlag("state", &turn.CurrentState); err != nil {
		return err
	}
	if err := setStringFlag("issues", &turn.KnownIssues); err != nil {
		return err
	}
	if err := setStringFlag("next", &turn.NextSteps); err != nil {
		return err
	}
	if err := setStringFlag("build-state", &turn.BuildState); err != nil {
		return err
	}
	if cmd.Flags().Changed("duration") {
		duration, err := cmd.Flags().GetInt("duration")
		if err != nil {
			return err
		}
		if duration < 0 {
			return fmt.Errorf("--duration must be >= 0")
		}
		turn.DurationSecs = duration
		updated++
	}

	if updated == 0 {
		return fmt.Errorf("no fields provided to update")
	}
	if err := s.UpdateTurn(turn); err != nil {
		return fmt.Errorf("updating turn #%d: %w", turn.ID, err)
	}

	fmt.Println()
	fmt.Printf("  %sTurn #%d updated.%s\n\n", styleBoldGreen, turn.ID, colorReset)
	return nil
}

func resolveTurnToUpdate(s *store.Store, args []string) (*store.Turn, error) {
	if len(args) > 0 {
		return findTurnByIdentifier(s, args[0])
	}

	if turnIDRaw := strings.TrimSpace(os.Getenv("ADAF_TURN_ID")); turnIDRaw != "" {
		turnID, err := strconv.Atoi(turnIDRaw)
		if err != nil || turnID <= 0 {
			return nil, fmt.Errorf("invalid ADAF_TURN_ID %q", turnIDRaw)
		}
		turn, err := s.GetTurn(turnID)
		if err != nil {
			return nil, fmt.Errorf("getting turn #%d from ADAF_TURN_ID: %w", turnID, err)
		}
		return turn, nil
	}

	latest, err := s.LatestTurn()
	if err != nil {
		return nil, fmt.Errorf("getting latest turn: %w", err)
	}
	if latest == nil {
		return nil, fmt.Errorf("no turns found to update")
	}
	return latest, nil
}

func findTurnByIdentifier(s *store.Store, rawID string) (*store.Turn, error) {
	id, err := strconv.Atoi(rawID)
	if err == nil {
		turn, getErr := s.GetTurn(id)
		if getErr != nil {
			return nil, fmt.Errorf("getting turn #%d: %w", id, getErr)
		}
		return turn, nil
	}

	hexID := strings.TrimSpace(rawID)
	turns, listErr := s.ListTurns()
	if listErr != nil {
		return nil, fmt.Errorf("listing turns: %w", listErr)
	}
	for i := range turns {
		if turns[i].HexID == hexID {
			return &turns[i], nil
		}
	}
	return nil, fmt.Errorf("turn not found for ID %q", rawID)
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

func runTurnSearch(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	query, _ := cmd.Flags().GetString("query")
	planFilter, _ := cmd.Flags().GetString("plan")
	agentFilter, _ := cmd.Flags().GetString("agent")
	limit, _ := cmd.Flags().GetInt("limit")

	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("--query is required")
	}
	queryLower := strings.ToLower(query)

	planFilter = strings.TrimSpace(planFilter)
	if planFilter != "" {
		if err := validatePlanID(planFilter); err != nil {
			return err
		}
	}
	agentFilter = strings.TrimSpace(strings.ToLower(agentFilter))

	turns, err := s.ListTurns()
	if err != nil {
		return fmt.Errorf("listing turns: %w", err)
	}

	var matches []store.Turn
	for _, t := range turns {
		if planFilter != "" && t.PlanID != planFilter {
			continue
		}
		if agentFilter != "" && strings.ToLower(t.Agent) != agentFilter {
			continue
		}
		if turnMatchesQuery(t, queryLower) {
			matches = append(matches, t)
		}
	}

	printHeader("Search Results")

	if len(matches) == 0 {
		fmt.Printf("  %sNo turns matching %q.%s\n\n", colorDim, query, colorReset)
		return nil
	}

	// Show most recent matches first, up to limit.
	if len(matches) > limit {
		matches = matches[len(matches)-limit:]
	}

	headers := []string{"ID", "DATE", "AGENT", "PLAN", "OBJECTIVE"}
	var rows [][]string
	for _, t := range matches {
		plan := "shared"
		if t.PlanID != "" {
			plan = t.PlanID
		}
		rows = append(rows, []string{
			fmt.Sprintf("#%d", t.ID),
			t.Date.Format("2006-01-02"),
			t.Agent,
			plan,
			truncate(t.Objective, 50),
		})
	}
	printTable(headers, rows)

	fmt.Printf("\n  %sMatches: %d (showing %d)%s\n\n", colorDim, len(matches), len(rows), colorReset)
	return nil
}

func turnMatchesQuery(t store.Turn, queryLower string) bool {
	fields := []string{
		t.Objective,
		t.WhatWasBuilt,
		t.KeyDecisions,
		t.NextSteps,
		t.Challenges,
		t.KnownIssues,
		t.CurrentState,
	}
	for _, f := range fields {
		if f != "" && strings.Contains(strings.ToLower(f), queryLower) {
			return true
		}
	}
	return false
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
