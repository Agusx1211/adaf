package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:     "issue",
	Aliases: []string{"issues", "bug", "bugs", "ticket", "tickets"},
	Short:   "Manage project issues",
	Long: `Create, list, show, and update project issues tracked by adaf.

Issues have a title, description, status (open, in_progress, resolved, wontfix),
priority (critical, high, medium, low), optional labels, and dependency links
to other issues. Issues are stored
as individual JSON files in the adaf project store.

Examples:
  adaf issue list                              # List all issues
  adaf issue list --status open                # Filter by status
  adaf issue create --title "Fix login bug" --priority high
  adaf issue show 3                            # Show issue details
  adaf issue update 3 --status resolved        # Mark as resolved`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var issueListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "l"},
	Short:   "List all issues",
	RunE:    runIssueList,
}

var issueCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new", "add", "open"},
	Short:   "Create a new issue",
	RunE:    runIssueCreate,
}

var issueShowCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get", "view", "display"},
	Short:   "Show issue details",
	Args:    cobra.ExactArgs(1),
	RunE:    runIssueShow,
}

var issueUpdateCmd = &cobra.Command{
	Use:     "update <id>",
	Aliases: []string{"edit", "modify", "set"},
	Short:   "Update an issue",
	Args:    cobra.ExactArgs(1),
	RunE:    runIssueUpdate,
}

func init() {
	issueListCmd.Flags().String("status", "", "Filter by status (open, in_progress, resolved, wontfix)")
	issueListCmd.Flags().String("plan", "", "Filter for a plan context (shared + plan-scoped)")
	issueListCmd.Flags().Bool("shared", false, "Show shared issues only")

	issueCreateCmd.Flags().String("title", "", "Issue title (required)")
	issueCreateCmd.Flags().String("description", "", "Issue description")
	issueCreateCmd.Flags().String("description-file", "", "Read description from file (use '-' for stdin)")
	issueCreateCmd.Flags().String("priority", "medium", "Priority (critical, high, medium, low)")
	issueCreateCmd.Flags().StringSlice("labels", nil, "Labels (comma-separated)")
	issueCreateCmd.Flags().IntSlice("depends-on", nil, "Issue IDs this issue depends on (comma-separated)")
	issueCreateCmd.Flags().String("plan", "", "Plan scope for this issue (empty = shared)")
	issueCreateCmd.Flags().Int("session", 0, "Associated turn ID (optional; defaults to current agent turn)")
	_ = issueCreateCmd.MarkFlagRequired("title")

	issueUpdateCmd.Flags().String("status", "", "New status")
	issueUpdateCmd.Flags().String("title", "", "New title")
	issueUpdateCmd.Flags().String("priority", "", "New priority")
	issueUpdateCmd.Flags().StringSlice("labels", nil, "Replace labels")
	issueUpdateCmd.Flags().IntSlice("depends-on", nil, "Replace dependency issue IDs (comma-separated)")
	issueUpdateCmd.Flags().String("plan", "", "Move issue to a plan scope (empty = shared)")

	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueShowCmd)
	issueCmd.AddCommand(issueUpdateCmd)
	rootCmd.AddCommand(issueCmd)
}

func runIssueList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	statusFilter, _ := cmd.Flags().GetString("status")
	planFilter, _ := cmd.Flags().GetString("plan")
	sharedOnly, _ := cmd.Flags().GetBool("shared")

	var issues []store.Issue
	switch {
	case sharedOnly:
		issues, err = s.ListSharedIssues()
	case strings.TrimSpace(planFilter) != "":
		planFilter = strings.TrimSpace(planFilter)
		if err := validatePlanID(planFilter); err != nil {
			return err
		}
		plan, getPlanErr := s.GetPlan(planFilter)
		if getPlanErr != nil {
			return fmt.Errorf("loading plan %q: %w", planFilter, getPlanErr)
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", planFilter)
		}
		issues, err = s.ListIssuesForPlan(planFilter)
	default:
		project, _ := s.LoadProject()
		if project != nil && project.ActivePlanID != "" {
			issues, err = s.ListIssuesForPlan(project.ActivePlanID)
		} else {
			issues, err = s.ListSharedIssues()
		}
	}
	if err != nil {
		return fmt.Errorf("listing issues: %w", err)
	}

	// Filter if needed
	if statusFilter != "" {
		var filtered []store.Issue
		for _, iss := range issues {
			if iss.Status == statusFilter {
				filtered = append(filtered, iss)
			}
		}
		issues = filtered
	}

	printHeader("Issues")

	if len(issues) == 0 {
		if statusFilter != "" {
			fmt.Printf("  %sNo issues with status %q.%s\n", colorDim, statusFilter, colorReset)
		} else {
			fmt.Printf("  %sNo issues found.%s\n", colorDim, colorReset)
		}
		fmt.Println()
		return nil
	}

	headers := []string{"ID", "STATUS", "PRI", "PLAN", "DEPS", "TITLE", "CREATED"}
	var rows [][]string
	for _, iss := range issues {
		scope := "shared"
		if iss.PlanID != "" {
			scope = iss.PlanID
		}
		rows = append(rows, []string{
			fmt.Sprintf("#%d", iss.ID),
			statusBadge(iss.Status),
			priorityBadge(iss.Priority),
			scope,
			formatIssueDependencyIDs(iss.DependsOn),
			truncate(iss.Title, 50),
			iss.Created.Format("2006-01-02"),
		})
	}
	printTable(headers, rows)

	fmt.Printf("\n  %sTotal: %d issue(s)%s\n\n", colorDim, len(issues), colorReset)
	return nil
}

func runIssueCreate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	title, _ := cmd.Flags().GetString("title")
	description, _ := cmd.Flags().GetString("description")
	descriptionFile, _ := cmd.Flags().GetString("description-file")
	priority, _ := cmd.Flags().GetString("priority")
	labels, _ := cmd.Flags().GetStringSlice("labels")
	dependsOn, _ := cmd.Flags().GetIntSlice("depends-on")
	planID, _ := cmd.Flags().GetString("plan")
	descriptionFile = strings.TrimSpace(descriptionFile)
	if description == "-" && descriptionFile == "" {
		descriptionFile = "-"
	}
	description, err = resolveTextFlag(description, descriptionFile)
	if err != nil {
		return fmt.Errorf("resolving description: %w", err)
	}
	planID = strings.TrimSpace(planID)
	turnFlag, _ := cmd.Flags().GetInt("session")
	turnID, err := resolveOptionalTurnID(turnFlag)
	if err != nil {
		return err
	}
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

	// Validate priority
	validPriorities := map[string]bool{
		"critical": true,
		"high":     true,
		"medium":   true,
		"low":      true,
	}
	if !validPriorities[priority] {
		return fmt.Errorf("invalid priority %q (valid: critical, high, medium, low)", priority)
	}

	normalizedDependsOn, err := s.ValidateIssueDependencies(0, dependsOn)
	if err != nil {
		return fmt.Errorf("validating dependencies: %w", err)
	}

	if client := TryConnect(); client != nil {
		projectID := projectIDFromPath(s.ProjectDir())
		request := map[string]interface{}{
			"title":       title,
			"description": description,
			"priority":    priority,
			"plan_id":     planID,
			"status":      "open",
			"labels":      labels,
			"depends_on":  normalizedDependsOn,
		}
		if turnID > 0 {
			request["turn_id"] = turnID
		}
		if err := client.CreateIssue(projectID, request); err == nil {
			fmt.Println()
			fmt.Printf("  %sIssue created via daemon.%s\n", styleBoldGreen, colorReset)
			printField("Title", title)
			printField("Priority", priority)
			printField("Status", "open")
			if planID != "" {
				printField("Plan", planID)
			} else {
				printField("Plan", "shared")
			}
			if turnID > 0 {
				printField("Turn", fmt.Sprintf("#%d", turnID))
			}
			if len(labels) > 0 {
				printField("Labels", strings.Join(labels, ", "))
			}
			if len(normalizedDependsOn) > 0 {
				printField("Depends On", formatIssueDependencyIDs(normalizedDependsOn))
			}
			fmt.Println()
			return nil
		}
	}

	issue := &store.Issue{
		Title:       title,
		Description: description,
		Priority:    priority,
		Labels:      labels,
		DependsOn:   normalizedDependsOn,
		PlanID:      planID,
		TurnID:      turnID,
	}

	if err := s.CreateIssue(issue); err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sIssue #%d created.%s\n", styleBoldGreen, issue.ID, colorReset)
	printField("Title", issue.Title)
	printField("Priority", issue.Priority)
	printField("Status", issue.Status)
	if issue.PlanID != "" {
		printField("Plan", issue.PlanID)
	} else {
		printField("Plan", "shared")
	}
	if issue.TurnID > 0 {
		printField("Turn", fmt.Sprintf("#%d", issue.TurnID))
	}
	if len(issue.Labels) > 0 {
		printField("Labels", strings.Join(issue.Labels, ", "))
	}
	if len(issue.DependsOn) > 0 {
		printField("Depends On", formatIssueDependencyIDs(issue.DependsOn))
	}
	fmt.Println()

	return nil
}

func runIssueShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue ID %q: must be a number", args[0])
	}

	issue, err := s.GetIssue(id)
	if err != nil {
		return fmt.Errorf("getting issue #%d: %w", id, err)
	}

	printHeader(fmt.Sprintf("Issue #%d", issue.ID))
	printField("Title", issue.Title)
	printFieldColored("Status", issue.Status, statusColor(issue.Status))
	printFieldColored("Priority", issue.Priority, statusColor(issue.Priority))
	printField("Created", issue.Created.Format("2006-01-02 15:04:05"))
	printField("Updated", issue.Updated.Format("2006-01-02 15:04:05"))
	if issue.PlanID != "" {
		printField("Plan", issue.PlanID)
	} else {
		printField("Plan", "shared")
	}

	if len(issue.Labels) > 0 {
		printField("Labels", strings.Join(issue.Labels, ", "))
	}
	if len(issue.DependsOn) > 0 {
		printField("Depends On", formatIssueDependencyIDs(issue.DependsOn))
	}
	if issue.TurnID > 0 {
		printField("Turn", fmt.Sprintf("#%d", issue.TurnID))
	}

	if issue.Description != "" {
		fmt.Println()
		fmt.Printf("  %sDescription:%s\n", colorBold, colorReset)
		for _, line := range strings.Split(issue.Description, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}

	fmt.Println()
	return nil
}

func runIssueUpdate(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue ID %q: must be a number", args[0])
	}

	issue, err := s.GetIssue(id)
	if err != nil {
		return fmt.Errorf("getting issue #%d: %w", id, err)
	}

	changed := false

	if cmd.Flags().Changed("status") {
		status, _ := cmd.Flags().GetString("status")
		validStatuses := map[string]bool{
			"open":        true,
			"in_progress": true,
			"resolved":    true,
			"wontfix":     true,
		}
		if !validStatuses[status] {
			return fmt.Errorf("invalid status %q (valid: open, in_progress, resolved, wontfix)", status)
		}
		issue.Status = status
		changed = true
	}

	if cmd.Flags().Changed("title") {
		title, _ := cmd.Flags().GetString("title")
		issue.Title = title
		changed = true
	}

	if cmd.Flags().Changed("priority") {
		priority, _ := cmd.Flags().GetString("priority")
		validPriorities := map[string]bool{
			"critical": true,
			"high":     true,
			"medium":   true,
			"low":      true,
		}
		if !validPriorities[priority] {
			return fmt.Errorf("invalid priority %q (valid: critical, high, medium, low)", priority)
		}
		issue.Priority = priority
		changed = true
	}

	if cmd.Flags().Changed("labels") {
		labels, _ := cmd.Flags().GetStringSlice("labels")
		issue.Labels = labels
		changed = true
	}

	if cmd.Flags().Changed("depends-on") {
		dependsOn, _ := cmd.Flags().GetIntSlice("depends-on")
		normalizedDependsOn, depErr := s.ValidateIssueDependencies(issue.ID, dependsOn)
		if depErr != nil {
			return fmt.Errorf("validating dependencies: %w", depErr)
		}
		issue.DependsOn = normalizedDependsOn
		changed = true
	}

	if cmd.Flags().Changed("plan") {
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
		issue.PlanID = planID
		changed = true
	}

	if !changed {
		return fmt.Errorf("no fields to update (use --status, --title, --priority, --labels, --depends-on, or --plan)")
	}

	if err := s.UpdateIssue(issue); err != nil {
		return fmt.Errorf("updating issue: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sIssue #%d updated.%s\n", styleBoldGreen, issue.ID, colorReset)
	printField("Title", issue.Title)
	printFieldColored("Status", issue.Status, statusColor(issue.Status))
	printFieldColored("Priority", issue.Priority, statusColor(issue.Priority))
	if issue.PlanID != "" {
		printField("Plan", issue.PlanID)
	} else {
		printField("Plan", "shared")
	}
	if len(issue.DependsOn) > 0 {
		printField("Depends On", formatIssueDependencyIDs(issue.DependsOn))
	}
	fmt.Println()

	return nil
}

func formatIssueDependencyIDs(dependsOn []int) string {
	if len(dependsOn) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(dependsOn))
	for _, id := range dependsOn {
		if id <= 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("#%d", id))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}
