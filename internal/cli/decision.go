package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var decisionCmd = &cobra.Command{
	Use:     "decision",
	Aliases: []string{"decisions", "adr", "adrs"},
	Short:   "Manage architectural decisions",
	Long: `Record, list, and view architectural decisions (ADRs) made during the project.

Each decision captures the context/problem, the decision made, the rationale,
and alternatives considered. Decisions are linked to sessions for traceability.

Examples:
  adaf decision list
  adaf decision show 3
  adaf decision create \
    --title "Use JWT for auth" \
    --context "Need stateless authentication" \
    --decision "Adopt JWT with RS256" \
    --rationale "Scales horizontally, no session store needed"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var decisionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all decisions",
	RunE:    runDecisionList,
}

var decisionCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add", "record"},
	Short:   "Record a new architectural decision",
	RunE:    runDecisionCreate,
}

var decisionShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show decision details",
	Args:    cobra.ExactArgs(1),
	RunE:    runDecisionShow,
}

func init() {
	decisionCreateCmd.Flags().String("title", "", "Decision title (required)")
	decisionCreateCmd.Flags().String("context", "", "Context/problem statement (required)")
	decisionCreateCmd.Flags().String("decision", "", "The decision made (required)")
	decisionCreateCmd.Flags().String("rationale", "", "Rationale for the decision (required)")
	decisionCreateCmd.Flags().String("alternatives", "", "Alternatives considered")
	decisionCreateCmd.Flags().Int("session", 0, "Associated session ID")
	_ = decisionCreateCmd.MarkFlagRequired("title")
	_ = decisionCreateCmd.MarkFlagRequired("context")
	_ = decisionCreateCmd.MarkFlagRequired("decision")
	_ = decisionCreateCmd.MarkFlagRequired("rationale")

	decisionCmd.AddCommand(decisionListCmd)
	decisionCmd.AddCommand(decisionCreateCmd)
	decisionCmd.AddCommand(decisionShowCmd)
	rootCmd.AddCommand(decisionCmd)
}

func runDecisionList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	decisions, err := s.ListDecisions()
	if err != nil {
		return fmt.Errorf("listing decisions: %w", err)
	}

	printHeader("Architectural Decisions")

	if len(decisions) == 0 {
		fmt.Printf("  %sNo decisions recorded.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"ID", "DATE", "TITLE", "SESSION"}
	var rows [][]string
	for _, d := range decisions {
		session := "-"
		if d.SessionID > 0 {
			session = fmt.Sprintf("#%d", d.SessionID)
		}
		rows = append(rows, []string{
			fmt.Sprintf("#%d", d.ID),
			d.Date.Format("2006-01-02"),
			truncate(d.Title, 50),
			session,
		})
	}
	printTable(headers, rows)

	fmt.Printf("\n  %sTotal: %d decision(s)%s\n\n", colorDim, len(decisions), colorReset)
	return nil
}

func runDecisionCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")
	context, _ := cmd.Flags().GetString("context")
	decision, _ := cmd.Flags().GetString("decision")
	rationale, _ := cmd.Flags().GetString("rationale")
	alternatives, _ := cmd.Flags().GetString("alternatives")
	sessionFlag, _ := cmd.Flags().GetInt("session")
	sessionID, err := resolveOptionalSessionID(sessionFlag)
	if err != nil {
		return err
	}

	dec := &store.Decision{
		Title:        title,
		Context:      context,
		Decision:     decision,
		Rationale:    rationale,
		Alternatives: alternatives,
		SessionID:    sessionID,
	}

	if err := s.CreateDecision(dec); err != nil {
		return fmt.Errorf("creating decision: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sDecision #%d recorded.%s\n", styleBoldGreen, dec.ID, colorReset)
	printField("Title", dec.Title)
	printField("Date", dec.Date.Format("2006-01-02 15:04:05"))
	fmt.Println()

	return nil
}

func runDecisionShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid decision ID %q: must be a number", args[0])
	}

	dec, err := s.GetDecision(id)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("decision #%d not found", id)
		}
		return fmt.Errorf("loading decision #%d: %w", id, err)
	}

	printHeader(fmt.Sprintf("Decision #%d", dec.ID))

	printField("Title", dec.Title)
	printField("Date", dec.Date.Format("2006-01-02 15:04:05"))
	if dec.SessionID > 0 {
		printField("Session", fmt.Sprintf("#%d", dec.SessionID))
	}

	fmt.Println()
	fmt.Printf("  %sContext:%s\n", colorBold, colorReset)
	for _, line := range strings.Split(dec.Context, "\n") {
		fmt.Printf("    %s\n", line)
	}

	fmt.Println()
	fmt.Printf("  %sDecision:%s\n", styleBoldGreen, colorReset)
	for _, line := range strings.Split(dec.Decision, "\n") {
		fmt.Printf("    %s\n", line)
	}

	fmt.Println()
	fmt.Printf("  %sRationale:%s\n", colorBold, colorReset)
	for _, line := range strings.Split(dec.Rationale, "\n") {
		fmt.Printf("    %s\n", line)
	}

	if dec.Alternatives != "" {
		fmt.Println()
		fmt.Printf("  %sAlternatives Considered:%s\n", colorDim, colorReset)
		for _, line := range strings.Split(dec.Alternatives, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}

	fmt.Println()
	return nil
}
