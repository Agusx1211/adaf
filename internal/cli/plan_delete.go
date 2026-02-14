package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runPlanDelete(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id := strings.TrimSpace(args[0])
	if err := validatePlanID(id); err != nil {
		return err
	}
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return fmt.Errorf("plan delete is destructive; rerun with --yes")
	}

	plan, err := s.GetPlan(id)
	if err != nil {
		return fmt.Errorf("loading plan %q: %w", id, err)
	}
	if plan == nil {
		return fmt.Errorf("plan %q not found", id)
	}
	if plan.Status != "done" && plan.Status != "cancelled" {
		return fmt.Errorf("plan %q is %q; only done/cancelled plans can be deleted", id, plan.Status)
	}

	if err := s.DeletePlan(id); err != nil {
		return fmt.Errorf("deleting plan: %w", err)
	}
	fmt.Printf("  %sPlan %s deleted.%s\n", styleBoldGreen, id, colorReset)
	return nil
}
