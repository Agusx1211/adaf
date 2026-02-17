package cli

import (
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:     "status",
	Aliases: []string{"info", "state", "st"},
	Short:   "Show comprehensive project status",
	Long: `Display a comprehensive summary of the project including plans, issues,
turn history, and wiki.`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	return runStatusFull(s)
}

// runStatusBrief is called when the root command runs with no subcommand.
func runStatusBrief(s *store.Store) error {
	config, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	activePlan, _ := s.ActivePlan()
	plans, _ := s.ListPlans()

	var issues []store.Issue
	if config.ActivePlanID != "" {
		issues, _ = s.ListIssuesForPlan(config.ActivePlanID)
	} else {
		issues, _ = s.ListSharedIssues()
	}
	openCount := 0
	for _, iss := range issues {
		if iss.Status == "open" || iss.Status == "in_progress" {
			openCount++
		}
	}

	fmt.Println()
	fmt.Printf("  %sadaf%s - %s%s%s\n", styleBoldCyan, colorReset, colorBold, config.Name, colorReset)
	fmt.Printf("  %s%s%s\n", colorDim, config.RepoPath, colorReset)
	fmt.Println()

	if openCount > 0 {
		fmt.Printf("  %s%d open issue(s)%s", colorYellow, openCount, colorReset)
	} else {
		fmt.Printf("  %sNo open issues%s", colorGreen, colorReset)
	}

	if len(plans) > 0 {
		fmt.Printf("  |  Plans: %d", len(plans))
	}
	if activePlan != nil {
		fmt.Printf("  |  Active: %s", activePlan.ID)
	} else if strings.TrimSpace(config.ActivePlanID) != "" {
		fmt.Printf("  |  Active: %s", config.ActivePlanID)
	} else {
		fmt.Printf("  |  Active: none")
	}

	turns, _ := s.ListTurns()
	fmt.Printf("  |  %d turn(s)\n", len(turns))

	fmt.Println()
	fmt.Printf("  Run %sadaf status%s for full details.\n\n", styleBoldWhite, colorReset)
	return nil
}

func runStatusFull(s *store.Store) error {
	config, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	plans, _ := s.ListPlans()
	activePlan, _ := s.ActivePlan()

	printHeader("Project")
	printField("Name", config.Name)
	printField("Repo", config.RepoPath)
	printField("Created", config.Created.Format("2006-01-02 15:04:05"))
	if config.ActivePlanID != "" {
		printField("Active Plan", config.ActivePlanID)
	} else {
		printField("Active Plan", "(none)")
	}

	printHeader("Plans")
	if len(plans) == 0 {
		fmt.Printf("  %sNo plans.%s\n", colorDim, colorReset)
	} else {
		headers := []string{"ID", "STATUS", "TITLE"}
		var rows [][]string
		for _, p := range plans {
			id := p.ID
			if p.ID == config.ActivePlanID {
				id = "● " + id
			}
			rows = append(rows, []string{
				id,
				statusBadge(p.Status),
				truncate(p.Title, 48),
			})
		}
		printTable(headers, rows)
	}

	printHeader("Active Plan")
	if activePlan == nil {
		fmt.Printf("  %sNo active plan selected.%s\n", colorDim, colorReset)
	} else {
		printField("ID", activePlan.ID)
		if activePlan.Title != "" {
			printField("Title", activePlan.Title)
		}
		printFieldColored("Status", activePlan.Status, statusColor(activePlan.Status))
		if strings.TrimSpace(activePlan.Description) != "" {
			printField("Description", truncate(activePlan.Description, 80))
		}
	}

	printHeader("Issues")
	if activePlan == nil {
		sharedIssues, _ := s.ListSharedIssues()
		printIssueSummary(sharedIssues, "shared")
	} else {
		visibleIssues, _ := s.ListIssuesForPlan(activePlan.ID)
		sharedIssues, _ := s.ListSharedIssues()
		sharedOpen := 0
		for _, iss := range sharedIssues {
			if iss.Status == "open" || iss.Status == "in_progress" {
				sharedOpen++
			}
		}
		printIssueSummary(visibleIssues, activePlan.ID)
		fmt.Printf("  %sShared open issues:%s %d\n", colorDim, colorReset, sharedOpen)
	}

	turns, _ := s.ListTurns()
	printHeader("Turns")
	fmt.Printf("  Total: %d turn(s)\n", len(turns))
	if len(turns) > 0 {
		latest := turns[len(turns)-1]
		fmt.Println()
		fmt.Printf("  %sLatest Turn (#%d):%s\n", colorBold, latest.ID, colorReset)
		printField("Date", latest.Date.Format("2006-01-02 15:04:05"))
		printField("Agent", latest.Agent)
		if latest.AgentModel != "" {
			printField("Model", latest.AgentModel)
		}
		if latest.PlanID != "" {
			printField("Plan", latest.PlanID)
		}
		printField("Objective", truncate(latest.Objective, 60))
		if latest.BuildState != "" {
			printField("Build State", latest.BuildState)
		}
		if latest.CommitHash != "" {
			printField("Last Commit", latest.CommitHash)
		}
	}

	printHeader("Wiki")
	sharedWiki, _ := s.ListSharedWiki()
	if activePlan == nil {
		fmt.Printf("  %d shared wiki entr", len(sharedWiki))
		if len(sharedWiki) == 1 {
			fmt.Printf("y\n")
		} else {
			fmt.Printf("ies\n")
		}
	} else {
		visibleWiki, _ := s.ListWikiForPlan(activePlan.ID)
		planScoped := 0
		for _, entry := range visibleWiki {
			if entry.PlanID == activePlan.ID {
				planScoped++
			}
		}
		fmt.Printf("  %d shared wiki entr", len(sharedWiki))
		if len(sharedWiki) == 1 {
			fmt.Printf("y")
		} else {
			fmt.Printf("ies")
		}
		fmt.Printf(", %d plan-scoped wiki entr", planScoped)
		if planScoped == 1 {
			fmt.Printf("y\n")
		} else {
			fmt.Printf("ies\n")
		}
	}

	fmt.Println()
	return nil
}

func printIssueSummary(issues []store.Issue, label string) {
	if len(issues) == 0 {
		fmt.Printf("  %sNo issues for %s.%s\n", colorDim, label, colorReset)
		return
	}

	counts := map[string]int{}
	for _, iss := range issues {
		counts[iss.Status]++
	}

	fmt.Printf("  ")
	if c := counts["open"]; c > 0 {
		fmt.Printf("%s%d open%s  ", colorBlue, c, colorReset)
	}
	if c := counts["in_progress"]; c > 0 {
		fmt.Printf("%s%d in-progress%s  ", colorYellow, c, colorReset)
	}
	if c := counts["resolved"]; c > 0 {
		fmt.Printf("%s%d resolved%s  ", colorGreen, c, colorReset)
	}
	if c := counts["wontfix"]; c > 0 {
		fmt.Printf("%s%d wontfix%s  ", colorDim, c, colorReset)
	}
	fmt.Printf("(%d total for %s)\n", len(issues), label)

	for _, iss := range issues {
		if (iss.Status == "open" || iss.Status == "in_progress") && (iss.Priority == "critical" || iss.Priority == "high") {
			fmt.Printf("  %s!%s #%d %s %s\n", colorRed, colorReset, iss.ID, priorityBadge(iss.Priority), iss.Title)
		}
	}
}
