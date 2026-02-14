package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runPlanPhaseStatus(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	phaseID := args[0]
	newStatus := args[1]

	validStatuses := map[string]bool{
		"not_started": true,
		"in_progress": true,
		"complete":    true,
		"blocked":     true,
	}
	if !validStatuses[newStatus] {
		return fmt.Errorf("invalid status %q (valid: not_started, in_progress, complete, blocked)", newStatus)
	}

	plan, err := s.ActivePlan()
	if err != nil {
		return fmt.Errorf("loading active plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("no active plan selected")
	}

	for i := range plan.Phases {
		if plan.Phases[i].ID != phaseID {
			continue
		}
		oldStatus := plan.Phases[i].Status
		plan.Phases[i].Status = newStatus
		if err := s.UpdatePlan(plan); err != nil {
			return fmt.Errorf("saving plan: %w", err)
		}

		fmt.Println()
		fmt.Printf("  Plan %s%s%s phase %s%s%s: %s -> %s\n",
			styleBoldWhite, plan.ID, colorReset,
			styleBoldWhite, phaseID, colorReset,
			statusBadge(oldStatus), statusBadge(newStatus))
		fmt.Println()
		return nil
	}

	return fmt.Errorf("phase %q not found in active plan %q", phaseID, plan.ID)
}
