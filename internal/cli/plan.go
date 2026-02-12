package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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

func runPlanCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id, _ := cmd.Flags().GetString("id")
	title, _ := cmd.Flags().GetString("title")
	description, _ := cmd.Flags().GetString("description")
	filePath, _ := cmd.Flags().GetString("file")
	id = strings.TrimSpace(id)

	if err := validatePlanID(id); err != nil {
		return err
	}

	plan := &store.Plan{
		ID:          id,
		Title:       strings.TrimSpace(title),
		Description: description,
		Status:      "active",
	}

	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading plan file %s: %w", filePath, err)
		}
		if err := json.Unmarshal(data, plan); err != nil {
			return fmt.Errorf("parsing plan JSON: %w", err)
		}
		plan.ID = id
		if strings.TrimSpace(title) != "" {
			plan.Title = strings.TrimSpace(title)
		}
		if description != "" {
			plan.Description = description
		}
		if plan.Status == "" {
			plan.Status = "active"
		}
	}

	if strings.TrimSpace(plan.Title) == "" {
		return fmt.Errorf("plan title is required (use --title or include title in --file)")
	}

	if err := s.CreatePlan(plan); err != nil {
		return fmt.Errorf("creating plan: %w", err)
	}

	project, _ := s.LoadProject()
	if project != nil && project.ActivePlanID == "" {
		_ = s.SetActivePlan(plan.ID)
	}

	fmt.Println()
	fmt.Printf("  %sPlan %s created.%s\n", styleBoldGreen, plan.ID, colorReset)
	printField("Title", plan.Title)
	printField("Status", plan.Status)
	printField("Phases", fmt.Sprintf("%d", len(plan.Phases)))
	fmt.Println()
	return nil
}

func runPlanSet(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	idOverride, _ := cmd.Flags().GetString("id")
	idOverride = strings.TrimSpace(idOverride)
	if idOverride != "" {
		if err := validatePlanID(idOverride); err != nil {
			return err
		}
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

	project, _ := s.LoadProject()
	active, _ := s.ActivePlan()

	targetID := idOverride
	if targetID == "" {
		targetID = strings.TrimSpace(plan.ID)
	}
	if targetID == "" && active != nil {
		targetID = active.ID
	}
	if targetID == "" && project != nil && project.ActivePlanID != "" {
		targetID = project.ActivePlanID
	}
	if targetID == "" {
		targetID = "default"
	}
	if err := validatePlanID(targetID); err != nil {
		return err
	}
	plan.ID = targetID

	existing, err := s.GetPlan(targetID)
	if err != nil {
		return fmt.Errorf("loading plan %q: %w", targetID, err)
	}
	if existing == nil {
		if plan.Status == "" {
			plan.Status = "active"
		}
		if err := s.CreatePlan(&plan); err != nil {
			return fmt.Errorf("creating plan: %w", err)
		}
	} else {
		if plan.Created.IsZero() {
			plan.Created = existing.Created
		}
		if err := s.UpdatePlan(&plan); err != nil {
			return fmt.Errorf("updating plan: %w", err)
		}
	}

	if project != nil && project.ActivePlanID == "" {
		_ = s.SetActivePlan(targetID)
	}

	fmt.Println()
	fmt.Printf("  %sPlan %s saved.%s\n", styleBoldGreen, targetID, colorReset)
	printField("Title", plan.Title)
	printField("Phases", fmt.Sprintf("%d", len(plan.Phases)))
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
	mergedDocs := 0
	closedIssues := 0

	switch newStatus {
	case "done":
		mergedIssues, mergedDocs, err = mergePlanToShared(s, planID)
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
		fmt.Printf("  %sMerged:%s %d issue(s), %d doc(s) to shared scope\n", colorDim, colorReset, mergedIssues, mergedDocs)
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

	docs, err := s.ListDocs()
	if err != nil {
		return 0, 0, fmt.Errorf("listing docs: %w", err)
	}
	mergedDocs := 0
	for i := range docs {
		doc := docs[i]
		if doc.PlanID != planID {
			continue
		}
		doc.PlanID = ""
		if err := s.UpdateDoc(&doc); err != nil {
			return 0, 0, fmt.Errorf("updating doc %s: %w", doc.ID, err)
		}
		mergedDocs++
	}

	return mergedIssues, mergedDocs, nil
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
