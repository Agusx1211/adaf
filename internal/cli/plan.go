package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage the project plan",
	Long:  `View and manage the project plan, including phases and their statuses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default to showing the plan
		return runPlanShow(cmd, args)
	},
}

var planShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the current plan",
	RunE:  runPlanShow,
}

var planSetCmd = &cobra.Command{
	Use:   "set [file]",
	Short: "Set the plan from a JSON file or stdin",
	Long: `Set the project plan from a JSON file. If no file is specified, reads from stdin.

The JSON should have the structure:
{
  "title": "...",
  "description": "...",
  "phases": [
    {"id": "p1", "title": "...", "description": "...", "status": "not_started", "priority": 1}
  ]
}`,
	RunE: runPlanSet,
}

var planPhaseStatusCmd = &cobra.Command{
	Use:   "phase-status <phase-id> <status>",
	Short: "Update a phase's status",
	Long:  `Update the status of a specific plan phase. Valid statuses: not_started, in_progress, complete, blocked`,
	Args:  cobra.ExactArgs(2),
	RunE:  runPlanPhaseStatus,
}

func init() {
	planCmd.AddCommand(planShowCmd)
	planCmd.AddCommand(planSetCmd)
	planCmd.AddCommand(planPhaseStatusCmd)
	rootCmd.AddCommand(planCmd)
}

func runPlanShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	plan, err := s.LoadPlan()
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	if plan.Title == "" && len(plan.Phases) == 0 {
		fmt.Println()
		fmt.Printf("  %sNo plan set.%s Run %sadaf plan set <file>%s to create one.\n",
			colorDim, colorReset, styleBoldWhite, colorReset)
		fmt.Println()
		return nil
	}

	printHeader("Project Plan")

	if plan.Title != "" {
		printField("Title", plan.Title)
	}
	if plan.Description != "" {
		printField("Description", plan.Description)
	}
	printField("Updated", plan.Updated.Format("2006-01-02 15:04:05"))

	// Phase summary counts
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

	// Phase table
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

	// Critical path
	if len(plan.CriticalPath) > 0 {
		fmt.Println()
		fmt.Printf("  %sCritical Path:%s %s\n", colorBold, colorReset, fmt.Sprintf("%v", plan.CriticalPath))
	}

	fmt.Println()
	return nil
}

func runPlanSet(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	var data []byte
	if len(args) > 0 {
		data, err = os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading plan file %s: %w", args[0], err)
		}
	} else {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
	}

	var plan store.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return fmt.Errorf("parsing plan JSON: %w", err)
	}

	if err := s.SavePlan(&plan); err != nil {
		return fmt.Errorf("saving plan: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sPlan saved.%s\n", styleBoldGreen, colorReset)
	printField("Title", plan.Title)
	printField("Phases", fmt.Sprintf("%d", len(plan.Phases)))
	fmt.Println()

	return nil
}

func runPlanPhaseStatus(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	phaseID := args[0]
	newStatus := args[1]

	// Validate status
	validStatuses := map[string]bool{
		"not_started": true,
		"in_progress": true,
		"complete":    true,
		"blocked":     true,
	}
	if !validStatuses[newStatus] {
		return fmt.Errorf("invalid status %q (valid: not_started, in_progress, complete, blocked)", newStatus)
	}

	plan, err := s.LoadPlan()
	if err != nil {
		return fmt.Errorf("loading plan: %w", err)
	}

	found := false
	for i, p := range plan.Phases {
		if p.ID == phaseID {
			oldStatus := plan.Phases[i].Status
			plan.Phases[i].Status = newStatus
			found = true

			if err := s.SavePlan(plan); err != nil {
				return fmt.Errorf("saving plan: %w", err)
			}

			fmt.Println()
			fmt.Printf("  Phase %s%s%s: %s -> %s\n",
				styleBoldWhite, phaseID, colorReset,
				statusBadge(oldStatus), statusBadge(newStatus))
			fmt.Println()
			return nil
		}
	}

	if !found {
		return fmt.Errorf("phase %q not found in plan", phaseID)
	}

	return nil
}
