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
turn history, and documents.`,
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
		done := 0
		for _, p := range activePlan.Phases {
			if p.Status == "complete" {
				done++
			}
		}
		fmt.Printf("  |  Active: %s (%d/%d phases)", activePlan.ID, done, len(activePlan.Phases))
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
			if p.ID == config.ActivePlanID {
				id = "● " + id
			}
			rows = append(rows, []string{
				id,
				statusBadge(p.Status),
				fmt.Sprintf("%d/%d", complete, len(p.Phases)),
				truncate(p.Title, 48),
			})
		}
		printTable(headers, rows)
	}

	printHeader("Active Plan")
	if activePlan == nil {
		fmt.Printf("  %sNo active plan selected.%s\n", colorDim, colorReset)
	} else {
		complete := 0
		inProgress := 0
		blocked := 0
		notStarted := 0
		for _, p := range activePlan.Phases {
			switch p.Status {
			case "complete":
				complete++
			case "in_progress":
				inProgress++
			case "blocked":
				blocked++
			default:
				notStarted++
			}
		}
		total := len(activePlan.Phases)
		printField("ID", activePlan.ID)
		if activePlan.Title != "" {
			printField("Title", activePlan.Title)
		}
		printFieldColored("Status", activePlan.Status, statusColor(activePlan.Status))
		if total > 0 {
			barWidth := 30
			filled := barWidth * complete / total
			fmt.Printf("  Progress:      [%s%s%s%s] %d/%d phases\n",
				colorGreen, repeatChar('#', filled), colorDim, repeatChar('-', barWidth-filled), complete, total)
			fmt.Print(colorReset)
		}
		fmt.Printf("  %s%d complete%s  %s%d in-progress%s  %s%d blocked%s  %s%d not-started%s\n",
			colorGreen, complete, colorReset,
			colorYellow, inProgress, colorReset,
			colorRed, blocked, colorReset,
			colorBlue, notStarted, colorReset,
		)
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

	printHeader("Documents")
	sharedDocs, _ := s.ListSharedDocs()
	if activePlan == nil {
		fmt.Printf("  %d shared document(s)\n", len(sharedDocs))
	} else {
		visibleDocs, _ := s.ListDocsForPlan(activePlan.ID)
		planScoped := 0
		for _, d := range visibleDocs {
			if d.PlanID == activePlan.ID {
				planScoped++
			}
		}
		fmt.Printf("  %d shared doc(s), %d plan-scoped doc(s)\n", len(sharedDocs), planScoped)
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

func repeatChar(ch byte, count int) string {
	if count <= 0 {
		return ""
	}
	b := make([]byte, count)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}
