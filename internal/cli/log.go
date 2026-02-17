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
  adaf turn finish --built "Added JWT auth" --decisions "Kept single-file flow" --challenges "Fixed toggle regression" --state "Demo restored" --issues "None" --next "Add refresh tokens"`,
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

var turnFinishCmd = &cobra.Command{
	Use:     "finish [id]",
	Aliases: []string{"complete"},
	Short:   "Publish final handoff report for a turn",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runTurnFinish,
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
	turnCmd.Flags().String("plan", "", "Filter turns by plan ID")
	turnCmd.Flags().Bool("all", false, "Include spawned sub-agent turns")
	turnListCmd.Flags().String("plan", "", "Filter turns by plan ID")
	turnListCmd.Flags().Bool("all", false, "Include spawned sub-agent turns")
	turnLatestCmd.Flags().Bool("all", false, "Include spawned sub-agent turns")

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

	addTurnFinishFlags(turnFinishCmd)

	turnSearchCmd.Flags().String("query", "", "Search query (required)")
	turnSearchCmd.Flags().String("plan", "", "Filter by plan ID")
	turnSearchCmd.Flags().String("agent", "", "Filter by agent name")
	turnSearchCmd.Flags().Int("limit", 10, "Maximum number of results")
	turnSearchCmd.Flags().Bool("all", false, "Include spawned sub-agent turns")
	_ = turnSearchCmd.MarkFlagRequired("query")

	turnCmd.AddCommand(turnListCmd)
	turnCmd.AddCommand(turnShowCmd)
	turnCmd.AddCommand(turnLatestCmd)
	turnCmd.AddCommand(turnCreateCmd)
	turnCmd.AddCommand(turnFinishCmd)
	turnCmd.AddCommand(turnSearchCmd)
	rootCmd.AddCommand(turnCmd)
}

func addTurnFinishFlags(cmd *cobra.Command) {
	cmd.Flags().String("objective", "", "Turn objective override")
	cmd.Flags().String("built", "", "What was built (required)")
	cmd.Flags().String("decisions", "", "Key decisions made (required)")
	cmd.Flags().String("challenges", "", "Challenges encountered (required)")
	cmd.Flags().String("state", "", "Current state of the project (required)")
	cmd.Flags().String("issues", "", "Known issues (required)")
	cmd.Flags().String("next", "", "Next steps (required)")
	_ = cmd.MarkFlagRequired("built")
	_ = cmd.MarkFlagRequired("decisions")
	_ = cmd.MarkFlagRequired("challenges")
	_ = cmd.MarkFlagRequired("state")
	_ = cmd.MarkFlagRequired("issues")
	_ = cmd.MarkFlagRequired("next")
}

func runTurnList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	planFilter, _ := cmd.Flags().GetString("plan")
	includeSpawned, _ := cmd.Flags().GetBool("all")
	planFilter = strings.TrimSpace(planFilter)
	if planFilter != "" {
		if err := validatePlanID(planFilter); err != nil {
			return err
		}
	}

	turns, err := listTurnsForLogs(s, includeSpawned)
	if err != nil {
		return err
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

	includeSpawned, _ := cmd.Flags().GetBool("all")
	turns, err := listTurnsForLogs(s, includeSpawned)
	if err != nil {
		return err
	}

	if len(turns) == 0 {
		fmt.Println()
		fmt.Printf("  %sNo turns found.%s\n\n", colorDim, colorReset)
		return nil
	}

	turn := turns[len(turns)-1]
	printTurn(&turn)
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

func runTurnFinish(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	turn, err := resolveTurnToUpdate(s, args)
	if err != nil {
		return err
	}
	if store.IsTurnFrozen(turn) {
		return fmt.Errorf("turn #%d is frozen and cannot be finished", turn.ID)
	}

	type finishSection struct {
		flag   string
		target *string
	}
	sections := []finishSection{
		{flag: "built", target: &turn.WhatWasBuilt},
		{flag: "decisions", target: &turn.KeyDecisions},
		{flag: "challenges", target: &turn.Challenges},
		{flag: "state", target: &turn.CurrentState},
		{flag: "issues", target: &turn.KnownIssues},
		{flag: "next", target: &turn.NextSteps},
	}

	values := make(map[string]string, len(sections))
	missing := make([]string, 0, len(sections))
	for _, sec := range sections {
		if !cmd.Flags().Changed(sec.flag) {
			missing = append(missing, "--"+sec.flag)
			continue
		}
		value, err := cmd.Flags().GetString(sec.flag)
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" {
			missing = append(missing, "--"+sec.flag)
			continue
		}
		values[sec.flag] = value
	}
	if len(missing) > 0 {
		return fmt.Errorf("turn finish requires all sections; missing: %s", strings.Join(missing, ", "))
	}

	wasComplete := turnHasCompleteFinishReport(turn)

	if cmd.Flags().Changed("objective") {
		objective, err := cmd.Flags().GetString("objective")
		if err != nil {
			return err
		}
		if strings.TrimSpace(objective) == "" {
			return fmt.Errorf("--objective cannot be empty when provided")
		}
		turn.Objective = objective
	}
	for _, sec := range sections {
		*sec.target = values[sec.flag]
	}

	if err := s.UpdateTurn(turn); err != nil {
		return fmt.Errorf("updating turn #%d: %w", turn.ID, err)
	}

	fmt.Println()
	if wasComplete {
		fmt.Printf("  %sTurn #%d finish report saved and overwrote the previous finish report.%s\n\n", styleBoldGreen, turn.ID, colorReset)
	} else {
		fmt.Printf("  %sTurn #%d finished with a complete handoff report.%s\n\n", styleBoldGreen, turn.ID, colorReset)
	}
	return nil
}

func turnHasCompleteFinishReport(turn *store.Turn) bool {
	if turn == nil {
		return false
	}
	required := []string{
		turn.WhatWasBuilt,
		turn.KeyDecisions,
		turn.Challenges,
		turn.CurrentState,
		turn.KnownIssues,
		turn.NextSteps,
	}
	for _, section := range required {
		if strings.TrimSpace(section) == "" {
			return false
		}
	}
	return true
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

	turns, err := listTurnsForLogs(s, false)
	if err != nil {
		return nil, fmt.Errorf("getting latest turn: %w", err)
	}
	if len(turns) == 0 {
		return nil, fmt.Errorf("no turns found to finish")
	}
	latest := turns[len(turns)-1]
	return &latest, nil
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
	includeSpawned, _ := cmd.Flags().GetBool("all")

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

	turns, err := listTurnsForLogs(s, includeSpawned)
	if err != nil {
		return err
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

func listTurnsForLogs(s *store.Store, includeSpawned bool) ([]store.Turn, error) {
	turns, err := s.ListTurns()
	if err != nil {
		return nil, fmt.Errorf("listing turns: %w", err)
	}
	if includeSpawned || len(turns) == 0 {
		return turns, nil
	}

	spawnedTurnIDs, err := childTurnIDSetFromSpawns(s)
	if err != nil {
		return nil, err
	}
	if len(spawnedTurnIDs) == 0 {
		return turns, nil
	}

	filtered := make([]store.Turn, 0, len(turns))
	for _, turn := range turns {
		if _, isSpawned := spawnedTurnIDs[turn.ID]; isSpawned {
			continue
		}
		filtered = append(filtered, turn)
	}
	return filtered, nil
}

func childTurnIDSetFromSpawns(s *store.Store) (map[int]struct{}, error) {
	spawns, err := s.ListSpawns()
	if err != nil {
		return nil, fmt.Errorf("listing spawns: %w", err)
	}
	if len(spawns) == 0 {
		return nil, nil
	}
	ids := make(map[int]struct{}, len(spawns))
	for _, rec := range spawns {
		if rec.ChildTurnID > 0 {
			ids[rec.ChildTurnID] = struct{}{}
		}
	}
	return ids, nil
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
