package cli

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

func runPlanStatus(cmd *cobra.Command, args []string) error {
	return runPlanStatusChange(args[0], args[1])
}

func runPlanStatusChange(planID, newStatus string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	planID = strings.TrimSpace(planID)
	newStatus = strings.TrimSpace(newStatus)
	if err := validatePlanID(planID); err != nil {
		return err
	}
	if !isValidPlanLifecycleStatus(newStatus) {
		return fmt.Errorf("invalid status %q (valid: active, frozen, done, cancelled)", newStatus)
	}

	plan, err := s.GetPlan(planID)
	if err != nil {
		return fmt.Errorf("loading plan %q: %w", planID, err)
	}
	if plan == nil {
		return fmt.Errorf("plan %q not found", planID)
	}

	oldStatus := plan.Status
	if oldStatus == "" {
		oldStatus = "active"
	}
	if oldStatus == newStatus {
		fmt.Printf("  %sPlan %s is already %s.%s\n", colorDim, planID, newStatus, colorReset)
		return nil
	}

	plan.Status = newStatus
	if err := s.UpdatePlan(plan); err != nil {
		return fmt.Errorf("updating plan status: %w", err)
	}

	mergedIssues := 0
	mergedWiki := 0
	closedIssues := 0

	switch newStatus {
	case "done":
		mergedIssues, mergedWiki, err = mergePlanToShared(s, planID)
		if err != nil {
			return err
		}
	case "cancelled":
		closedIssues, err = closePlanIssuesAsWontfix(s, planID)
		if err != nil {
			return err
		}
	}

	project, _ := s.LoadProject()
	if project != nil && project.ActivePlanID == planID && newStatus != "active" {
		_ = s.SetActivePlan("")
	}

	fmt.Println()
	fmt.Printf("  Plan %s%s%s: %s -> %s\n", styleBoldWhite, planID, colorReset, statusBadge(oldStatus), statusBadge(newStatus))
	if newStatus == "done" {
		fmt.Printf("  %sMerged:%s %d issue(s), %d wiki entr", colorDim, colorReset, mergedIssues, mergedWiki)
		if mergedWiki == 1 {
			fmt.Printf("y to shared scope\n")
		} else {
			fmt.Printf("ies to shared scope\n")
		}
	}
	if newStatus == "cancelled" {
		fmt.Printf("  %sClosed:%s %d issue(s) as wontfix\n", colorDim, colorReset, closedIssues)
	}
	fmt.Println()

	return nil
}

func mergePlanToShared(s *store.Store, planID string) (int, int, error) {
	issues, err := s.ListIssues()
	if err != nil {
		return 0, 0, fmt.Errorf("listing issues: %w", err)
	}
	mergedIssues := 0
	for i := range issues {
		iss := issues[i]
		if iss.PlanID != planID {
			continue
		}
		if iss.Status != "open" && iss.Status != "in_progress" {
			continue
		}
		iss.PlanID = ""
		if err := s.UpdateIssue(&iss); err != nil {
			return 0, 0, fmt.Errorf("updating issue #%d: %w", iss.ID, err)
		}
		mergedIssues++
	}

	wiki, err := s.ListWiki()
	if err != nil {
		return 0, 0, fmt.Errorf("listing wiki: %w", err)
	}
	mergedWiki := 0
	for i := range wiki {
		entry := wiki[i]
		if entry.PlanID != planID {
			continue
		}
		entry.PlanID = ""
		if err := s.UpdateWikiEntry(&entry); err != nil {
			return 0, 0, fmt.Errorf("updating wiki entry %s: %w", entry.ID, err)
		}
		mergedWiki++
	}

	return mergedIssues, mergedWiki, nil
}

func closePlanIssuesAsWontfix(s *store.Store, planID string) (int, error) {
	issues, err := s.ListIssues()
	if err != nil {
		return 0, fmt.Errorf("listing issues: %w", err)
	}

	closed := 0
	for i := range issues {
		iss := issues[i]
		if iss.PlanID != planID {
			continue
		}
		if iss.Status != "open" && iss.Status != "in_progress" {
			continue
		}
		iss.Status = "wontfix"
		if err := s.UpdateIssue(&iss); err != nil {
			return 0, fmt.Errorf("updating issue #%d: %w", iss.ID, err)
		}
		closed++
	}
	return closed, nil
}

func isValidPlanLifecycleStatus(status string) bool {
	switch status {
	case "active", "frozen", "done", "cancelled":
		return true
	default:
		return false
	}
}
