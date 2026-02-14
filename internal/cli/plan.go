package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var planIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

var planCmd = &cobra.Command{
	Use:     "plan",
	Aliases: []string{"plans"},
	Short:   "Manage project plans",
	Long: `View and manage project plans.

Examples:
  adaf plan
  adaf plan list
  adaf plan create --id auth-system --title "Authentication System"
  adaf plan switch auth-system
  adaf plan status auth-system frozen
  adaf plan phase-status p1 complete`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlanShow(cmd, args)
	},
}

var planListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all plans",
	RunE:    runPlanList,
}

var planShowCmd = &cobra.Command{
	Use:     "show [id]",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show a plan (defaults to active plan)",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runPlanShow,
}

var planCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new plan",
	RunE:  runPlanCreate,
}

var planSetCmd = &cobra.Command{
	Use:     "set [file]",
	Aliases: []string{"load", "import"},
	Short:   "Set plan content from JSON file or stdin",
	RunE:    runPlanSet,
}

var planSwitchCmd = &cobra.Command{
	Use:   "switch <id>",
	Short: "Set active plan",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanSwitch,
}

var planStatusCmd = &cobra.Command{
	Use:   "status <id> <status>",
	Short: "Set plan lifecycle status (active, frozen, done, cancelled)",
	Args:  cobra.ExactArgs(2),
	RunE:  runPlanStatus,
}

var planArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Alias for `plan status <id> done`",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlanStatusChange(args[0], "done")
	},
}

var planFreezeCmd = &cobra.Command{
	Use:   "freeze <id>",
	Short: "Alias for `plan status <id> frozen`",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlanStatusChange(args[0], "frozen")
	},
}

var planUnfreezeCmd = &cobra.Command{
	Use:   "unfreeze <id>",
	Short: "Alias for `plan status <id> active`",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlanStatusChange(args[0], "active")
	},
}

var planCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Alias for `plan status <id> cancelled`",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPlanStatusChange(args[0], "cancelled")
	},
}

var planDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a done/cancelled plan",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanDelete,
}

var planNoneCmd = &cobra.Command{
	Use:   "none",
	Short: "Clear active plan",
	RunE:  runPlanNone,
}

var planPhaseStatusCmd = &cobra.Command{
	Use:     "phase-status <phase-id> <status>",
	Aliases: []string{"phase_status", "update-phase", "update_phase"},
	Short:   "Update a phase status on the active plan",
	Args:    cobra.ExactArgs(2),
	RunE:    runPlanPhaseStatus,
}

func init() {
	planCreateCmd.Flags().String("id", "", "Plan ID (required)")
	planCreateCmd.Flags().String("title", "", "Plan title (required unless --file contains title)")
	planCreateCmd.Flags().String("description", "", "Plan description")
	planCreateCmd.Flags().String("file", "", "Load additional plan fields (phases, etc) from JSON file")
	_ = planCreateCmd.MarkFlagRequired("id")

	planSetCmd.Flags().String("id", "", "Target plan ID override")

	planDeleteCmd.Flags().Bool("yes", false, "Confirm destructive delete")

	planCmd.AddCommand(
		planListCmd,
		planShowCmd,
		planCreateCmd,
		planSetCmd,
		planSwitchCmd,
		planStatusCmd,
		planArchiveCmd,
		planFreezeCmd,
		planUnfreezeCmd,
		planCancelCmd,
		planDeleteCmd,
		planNoneCmd,
		planPhaseStatusCmd,
	)
	rootCmd.AddCommand(planCmd)
}

func validatePlanID(id string) error {
	if !planIDPattern.MatchString(id) {
		return fmt.Errorf("invalid plan ID %q (must match %s)", id, planIDPattern.String())
	}
	return nil
}

func runPlanList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	plans, err := s.ListPlans()
	if err != nil {
		return fmt.Errorf("listing plans: %w", err)
	}
	project, _ := s.LoadProject()
	activeID := ""
	if project != nil {
		activeID = project.ActivePlanID
	}

	printHeader("Plans")
	if len(plans) == 0 {
		fmt.Printf("  %sNo plans found.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"ID", "STATUS", "PHASES", "TITLE"}
	var rows [][]string
	for _, p := range plans {
		complete := 0
		for _, ph := range p.Phases {
			if ph.Status == "complete" {
				complete++
			}
		}
		id := p.ID
		if p.ID == activeID {
			id = "* " + id
		}
		rows = append(rows, []string{
			id,
			statusBadge(p.Status),
			fmt.Sprintf("%d/%d", complete, len(p.Phases)),
			truncate(p.Title, 48),
		})
	}
	printTable(headers, rows)

	if activeID != "" {
		fmt.Printf("\n  %sActive plan:%s %s\n", colorDim, colorReset, activeID)
	}
	fmt.Println()
	return nil
}

func runPlanShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	var plan *store.Plan
	if len(args) == 1 {
		if err := validatePlanID(args[0]); err != nil {
			return err
		}
		plan, err = s.GetPlan(args[0])
		if err != nil {
			return fmt.Errorf("loading plan %q: %w", args[0], err)
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", args[0])
		}
	} else {
		plan, err = s.ActivePlan()
		if err != nil {
			return fmt.Errorf("loading active plan: %w", err)
		}
		if plan == nil {
			fmt.Println()
			fmt.Printf("  %sNo active plan selected.%s Use %sadaf plan switch <id>%s.\n", colorDim, colorReset, styleBoldWhite, colorReset)
			fmt.Println()
			return nil
		}
	}

	printHeader("Plan")
	printField("ID", plan.ID)
	printFieldColored("Status", plan.Status, statusColor(plan.Status))
	if plan.Title != "" {
		printField("Title", plan.Title)
	}
	if plan.Description != "" {
		printField("Description", plan.Description)
	}
	if !plan.Created.IsZero() {
		printField("Created", plan.Created.Format("2006-01-02 15:04:05"))
	}
	if !plan.Updated.IsZero() {
		printField("Updated", plan.Updated.Format("2006-01-02 15:04:05"))
	}

	counts := map[string]int{}
	for _, p := range plan.Phases {
		counts[p.Status]++
	}

	fmt.Println()
	fmt.Printf("  %sPhase Summary:%s", colorBold, colorReset)
	if c := counts["complete"]; c > 0 {
		fmt.Printf("  %s%d complete%s", colorGreen, c, colorReset)
	}
	if c := counts["in_progress"]; c > 0 {
		fmt.Printf("  %s%d in-progress%s", colorYellow, c, colorReset)
	}
	if c := counts["blocked"]; c > 0 {
		fmt.Printf("  %s%d blocked%s", colorRed, c, colorReset)
	}
	if c := counts["not_started"]; c > 0 {
		fmt.Printf("  %s%d not-started%s", colorBlue, c, colorReset)
	}
	fmt.Println()

	if len(plan.Phases) > 0 {
		fmt.Println()
		headers := []string{"ID", "TITLE", "STATUS", "PRI", "DEPENDS"}
		var rows [][]string
		for _, p := range plan.Phases {
			deps := "-"
			if len(p.DependsOn) > 0 {
				deps = fmt.Sprintf("%v", p.DependsOn)
			}
			rows = append(rows, []string{
				p.ID,
				truncate(p.Title, 40),
				statusBadge(p.Status),
				fmt.Sprintf("%d", p.Priority),
				deps,
			})
		}
		printTable(headers, rows)
	}

	if len(plan.CriticalPath) > 0 {
		fmt.Println()
		fmt.Printf("  %sCritical Path:%s %v\n", colorBold, colorReset, plan.CriticalPath)
	}

	fmt.Println()
	return nil
}

func runPlanSwitch(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id := strings.TrimSpace(args[0])
	if err := validatePlanID(id); err != nil {
		return err
	}

	plan, err := s.GetPlan(id)
	if err != nil {
		return fmt.Errorf("loading plan %q: %w", id, err)
	}
	if plan == nil {
		return fmt.Errorf("plan %q not found", id)
	}
	if plan.Status != "" && plan.Status != "active" {
		return fmt.Errorf("plan %q is %q; only active plans can be selected", id, plan.Status)
	}

	if err := s.SetActivePlan(id); err != nil {
		return fmt.Errorf("setting active plan: %w", err)
	}

	fmt.Printf("  %sActive plan set to %s.%s\n", styleBoldGreen, id, colorReset)
	return nil
}

func runPlanNone(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	if err := s.SetActivePlan(""); err != nil {
		return fmt.Errorf("clearing active plan: %w", err)
	}
	fmt.Printf("  %sActive plan cleared.%s\n", styleBoldGreen, colorReset)
	return nil
}
