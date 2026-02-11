package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage project issues",
	Long:  `Create, list, show, and update project issues tracked by adaf.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all issues",
	RunE:  runIssueList,
}

var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	RunE:  runIssueCreate,
}

var issueShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show issue details",
	Args:  cobra.ExactArgs(1),
	RunE:  runIssueShow,
}

var issueUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update an issue",
	Args:  cobra.ExactArgs(1),
	RunE:  runIssueUpdate,
}

func init() {
	issueListCmd.Flags().String("status", "", "Filter by status (open, in_progress, resolved, wontfix)")

	issueCreateCmd.Flags().String("title", "", "Issue title (required)")
	issueCreateCmd.Flags().String("description", "", "Issue description")
	issueCreateCmd.Flags().String("priority", "medium", "Priority (critical, high, medium, low)")
	issueCreateCmd.Flags().StringSlice("labels", nil, "Labels (comma-separated)")
	_ = issueCreateCmd.MarkFlagRequired("title")

	issueUpdateCmd.Flags().String("status", "", "New status")
	issueUpdateCmd.Flags().String("title", "", "New title")
	issueUpdateCmd.Flags().String("priority", "", "New priority")
	issueUpdateCmd.Flags().StringSlice("labels", nil, "Replace labels")

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

	issues, err := s.ListIssues()
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

	headers := []string{"ID", "STATUS", "PRI", "TITLE", "CREATED"}
	var rows [][]string
	for _, iss := range issues {
		rows = append(rows, []string{
			fmt.Sprintf("#%d", iss.ID),
			statusBadge(iss.Status),
			priorityBadge(iss.Priority),
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
	priority, _ := cmd.Flags().GetString("priority")
	labels, _ := cmd.Flags().GetStringSlice("labels")

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

	issue := &store.Issue{
		Title:       title,
		Description: description,
		Priority:    priority,
		Labels:      labels,
	}

	if err := s.CreateIssue(issue); err != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sIssue #%d created.%s\n", styleBoldGreen, issue.ID, colorReset)
	printField("Title", issue.Title)
	printField("Priority", issue.Priority)
	printField("Status", issue.Status)
	if len(issue.Labels) > 0 {
		printField("Labels", strings.Join(issue.Labels, ", "))
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

	if len(issue.Labels) > 0 {
		printField("Labels", strings.Join(issue.Labels, ", "))
	}
	if issue.SessionID > 0 {
		printField("Session", fmt.Sprintf("#%d", issue.SessionID))
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

	if !changed {
		return fmt.Errorf("no fields to update (use --status, --title, --priority, or --labels)")
	}

	if err := s.UpdateIssue(issue); err != nil {
		return fmt.Errorf("updating issue: %w", err)
	}

	fmt.Println()
	fmt.Printf("  %sIssue #%d updated.%s\n", styleBoldGreen, issue.ID, colorReset)
	printField("Title", issue.Title)
	printFieldColored("Status", issue.Status, statusColor(issue.Status))
	printFieldColored("Priority", issue.Priority, statusColor(issue.Priority))
	fmt.Println()

	return nil
}
