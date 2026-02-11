package cli

import (
	"fmt"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show comprehensive project status",
	Long:  `Display a summary of the project including plan progress, issues, sessions, and recent activity.`,
	RunE:  runStatus,
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

	fmt.Println()
	fmt.Printf("  %sadaf%s - %s%s%s\n", styleBoldCyan, colorReset, colorBold, config.Name, colorReset)
	fmt.Printf("  %s%s%s\n", colorDim, config.RepoPath, colorReset)
	fmt.Println()

	// Quick issue count
	issues, _ := s.ListIssues()
	openCount := 0
	for _, iss := range issues {
		if iss.Status == "open" || iss.Status == "in_progress" {
			openCount++
		}
	}
	if openCount > 0 {
		fmt.Printf("  %s%d open issue(s)%s", colorYellow, openCount, colorReset)
	} else {
		fmt.Printf("  %sNo open issues%s", colorGreen, colorReset)
	}

	// Quick plan count
	plan, _ := s.LoadPlan()
	if plan != nil && len(plan.Phases) > 0 {
		done := 0
		for _, p := range plan.Phases {
			if p.Status == "complete" {
				done++
			}
		}
		fmt.Printf("  |  Plan: %d/%d phases complete", done, len(plan.Phases))
	}

	// Quick session count
	logs, _ := s.ListLogs()
	fmt.Printf("  |  %d session(s)\n", len(logs))

	fmt.Println()
	fmt.Printf("  Run %sadaf status%s for full details.\n\n", styleBoldWhite, colorReset)

	return nil
}

// runStatusFull shows the comprehensive project status.
func runStatusFull(s *store.Store) error {
	config, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	// === Project Info ===
	printHeader("Project")
	printField("Name", config.Name)
	printField("Repo", config.RepoPath)
	printField("Created", config.Created.Format("2006-01-02 15:04:05"))

	// === Plan ===
	plan, _ := s.LoadPlan()
	printHeader("Plan")

	if plan == nil || (plan.Title == "" && len(plan.Phases) == 0) {
		fmt.Printf("  %sNo plan set.%s\n", colorDim, colorReset)
	} else {
		if plan.Title != "" {
			printField("Title", plan.Title)
		}

		counts := map[string]int{}
		for _, p := range plan.Phases {
			counts[p.Status]++
		}
		total := len(plan.Phases)

		complete := counts["complete"]
		inProgress := counts["in_progress"]
		blocked := counts["blocked"]
		notStarted := counts["not_started"]

		// Progress bar
		if total > 0 {
			barWidth := 30
			filled := barWidth * complete / total
			fmt.Printf("  Progress:      [%s%s%s%s] %d/%d phases\n",
				colorGreen, repeatChar('#', filled), colorDim, repeatChar('-', barWidth-filled),
				complete, total)
			fmt.Print(colorReset)
		}

		fmt.Printf("  ")
		if complete > 0 {
			fmt.Printf("%s%d complete%s  ", colorGreen, complete, colorReset)
		}
		if inProgress > 0 {
			fmt.Printf("%s%d in-progress%s  ", colorYellow, inProgress, colorReset)
		}
		if blocked > 0 {
			fmt.Printf("%s%d blocked%s  ", colorRed, blocked, colorReset)
		}
		if notStarted > 0 {
			fmt.Printf("%s%d not-started%s  ", colorBlue, notStarted, colorReset)
		}
		fmt.Println()
	}

	// === Issues ===
	issues, _ := s.ListIssues()
	printHeader("Issues")

	if len(issues) == 0 {
		fmt.Printf("  %sNo issues.%s\n", colorDim, colorReset)
	} else {
		issueCounts := map[string]int{}
		for _, iss := range issues {
			issueCounts[iss.Status]++
		}

		fmt.Printf("  ")
		if c := issueCounts["open"]; c > 0 {
			fmt.Printf("%s%d open%s  ", colorBlue, c, colorReset)
		}
		if c := issueCounts["in_progress"]; c > 0 {
			fmt.Printf("%s%d in-progress%s  ", colorYellow, c, colorReset)
		}
		if c := issueCounts["resolved"]; c > 0 {
			fmt.Printf("%s%d resolved%s  ", colorGreen, c, colorReset)
		}
		if c := issueCounts["wontfix"]; c > 0 {
			fmt.Printf("%s%d wontfix%s  ", colorDim, c, colorReset)
		}
		fmt.Printf("(%d total)\n", len(issues))

		// Show critical/high priority open issues
		for _, iss := range issues {
			if (iss.Status == "open" || iss.Status == "in_progress") &&
				(iss.Priority == "critical" || iss.Priority == "high") {
				fmt.Printf("  %s!%s #%d %s %s\n",
					colorRed, colorReset, iss.ID, priorityBadge(iss.Priority), iss.Title)
			}
		}
	}

	// === Sessions ===
	logs, _ := s.ListLogs()
	printHeader("Sessions")

	fmt.Printf("  Total: %d session(s)\n", len(logs))

	if len(logs) > 0 {
		latest := logs[len(logs)-1]
		fmt.Println()
		fmt.Printf("  %sLatest Session (#%d):%s\n", colorBold, latest.ID, colorReset)
		printField("Date", latest.Date.Format("2006-01-02 15:04:05"))
		printField("Agent", latest.Agent)
		if latest.AgentModel != "" {
			printField("Model", latest.AgentModel)
		}
		printField("Objective", truncate(latest.Objective, 60))
		if latest.BuildState != "" {
			printField("Build State", latest.BuildState)
		}
		if latest.CommitHash != "" {
			printField("Last Commit", latest.CommitHash)
		}
	}

	// === Decisions ===
	decisions, _ := s.ListDecisions()
	if len(decisions) > 0 {
		printHeader("Decisions")
		fmt.Printf("  %d architectural decision(s) recorded.\n", len(decisions))
		// Show last 3
		start := 0
		if len(decisions) > 3 {
			start = len(decisions) - 3
		}
		for _, d := range decisions[start:] {
			fmt.Printf("  %s#%d%s %s %s(%s)%s\n",
				colorBold, d.ID, colorReset, d.Title, colorDim, d.Date.Format("2006-01-02"), colorReset)
		}
	}

	// === Docs ===
	docs, _ := s.ListDocs()
	if len(docs) > 0 {
		printHeader("Documents")
		fmt.Printf("  %d document(s)\n", len(docs))
		for _, d := range docs {
			fmt.Printf("  %s[%s]%s %s\n", colorCyan, d.ID, colorReset, d.Title)
		}
	}

	fmt.Println()
	return nil
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
