package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
)

var sessionsCmd = &cobra.Command{
	Use:     "sessions",
	Aliases: []string{"session-list", "session_list", "ls-sessions", "ls_sessions"},
	Short:   "List active and recent adaf sessions",
	Long: `List all adaf sessions that are currently running or recently completed.
Sessions are created when agents are launched through the TUI with session support.

Use 'adaf attach <id>' to reattach to a running session.`,
	RunE: runSessions,
}

func init() {
	sessionsCmd.Flags().Bool("all", false, "Show all sessions (including completed/dead)")
	rootCmd.AddCommand(sessionsCmd)
}

func runSessions(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("session management is not available inside an agent context")
	}

	showAll, _ := cmd.Flags().GetBool("all")

	// Clean up old sessions (older than 24 hours).
	session.CleanupOld(24 * time.Hour)

	sessions, err := session.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if !showAll {
		// Filter to only active + recently finished (last hour).
		cutoff := time.Now().Add(-1 * time.Hour)
		var filtered []session.SessionMeta
		for _, s := range sessions {
			if session.IsActiveStatus(s.Status) {
				filtered = append(filtered, s)
			} else if !s.EndedAt.IsZero() && s.EndedAt.After(cutoff) {
				filtered = append(filtered, s)
			} else if s.EndedAt.IsZero() && s.StartedAt.After(cutoff) {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	if len(sessions) == 0 {
		fmt.Println(colorDim + "  No active sessions." + colorReset)
		fmt.Println()
		fmt.Println("  Start a session from the TUI (" + styleBoldWhite + "adaf" + colorReset + ") or with " + styleBoldWhite + "adaf run --session" + colorReset)
		return nil
	}

	fmt.Println()
	fmt.Println(styleBoldCyan + "  Sessions" + colorReset)
	fmt.Println(colorDim + "  " + strings.Repeat("-", 40) + colorReset)
	fmt.Println()

	var rows [][]string
	for _, s := range sessions {
		status := formatSessionStatus(s.Status)
		elapsed := ""
		if session.IsActiveStatus(s.Status) {
			elapsed = session.FormatElapsed(time.Since(s.StartedAt))
		} else if !s.EndedAt.IsZero() {
			elapsed = session.FormatTimeAgo(s.EndedAt)
		} else {
			elapsed = session.FormatTimeAgo(s.StartedAt)
		}

		project := s.ProjectName
		if project == "" {
			project = "-"
		}

		loopName := s.LoopName
		if loopName == "" {
			loopName = "-"
		}

		rows = append(rows, []string{
			fmt.Sprintf("%d", s.ID),
			loopName,
			s.ProfileName,
			s.AgentName,
			project,
			status,
			elapsed,
		})
	}

	printTable(
		[]string{"ID", "LOOP", "Profile", "Agent", "Project", "Status", "Time"},
		rows,
	)

	fmt.Println()

	// Show hint for active sessions.
	hasActive := false
	for _, s := range sessions {
		if s.Status == session.StatusRunning {
			hasActive = true
			break
		}
	}
	if hasActive {
		fmt.Printf("  Use %sadaf attach [loop-name|id]%s to reattach to a running session.\n", styleBoldWhite, colorReset)
		fmt.Printf("  Use %sadaf sessions output [id]%s to inspect output without attaching.\n", styleBoldWhite, colorReset)
		fmt.Printf("  Use %sadaf sessions logs [id]%s to inspect adaf daemon logs.\n", styleBoldWhite, colorReset)
		fmt.Println()
	}

	return nil
}

func formatSessionStatus(status string) string {
	switch status {
	case session.StatusRunning:
		return styleBoldGreen + session.StatusRunning + colorReset
	case session.StatusStarting:
		return styleBoldYellow + session.StatusStarting + colorReset
	case session.StatusDone:
		return colorGreen + session.StatusDone + colorReset
	case session.StatusCancelled:
		return styleBoldYellow + session.StatusCancelled + colorReset
	case session.StatusError:
		return styleBoldRed + session.StatusError + colorReset
	case session.StatusDead:
		return colorDim + session.StatusDead + colorReset
	default:
		return status
	}
}
