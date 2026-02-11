package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage session recordings",
	Long:  `View and replay session recordings that capture agent interactions.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recorded sessions",
	RunE:  runSessionList,
}

var sessionShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show session recording details",
	Args:  cobra.ExactArgs(1),
	RunE:  runSessionShow,
}

func init() {
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	rootCmd.AddCommand(sessionCmd)
}

func runSessionList(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	// List recording directories
	recDir := filepath.Join(s.Root(), "recordings")
	entries, err := os.ReadDir(recDir)
	if err != nil {
		if os.IsNotExist(err) {
			printHeader("Session Recordings")
			fmt.Printf("  %sNo recordings found.%s\n\n", colorDim, colorReset)
			return nil
		}
		return fmt.Errorf("reading recordings directory: %w", err)
	}

	printHeader("Session Recordings")

	var rows [][]string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sessionID, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		rec, err := s.LoadRecording(sessionID)
		if err != nil {
			continue
		}

		duration := "-"
		if !rec.EndTime.IsZero() {
			d := rec.EndTime.Sub(rec.StartTime)
			duration = fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
		}

		exitCodeStr := fmt.Sprintf("%d", rec.ExitCode)
		if rec.ExitCode == 0 {
			exitCodeStr = colorGreen + "0" + colorReset
		} else {
			exitCodeStr = colorRed + exitCodeStr + colorReset
		}

		rows = append(rows, []string{
			fmt.Sprintf("#%d", rec.SessionID),
			rec.Agent,
			rec.StartTime.Format("2006-01-02 15:04"),
			duration,
			fmt.Sprintf("%d", len(rec.Events)),
			exitCodeStr,
		})
	}

	if len(rows) == 0 {
		fmt.Printf("  %sNo recordings found.%s\n\n", colorDim, colorReset)
		return nil
	}

	headers := []string{"SESSION", "AGENT", "STARTED", "DURATION", "EVENTS", "EXIT"}
	printTable(headers, rows)

	fmt.Printf("\n  %sTotal: %d recording(s)%s\n\n", colorDim, len(rows), colorReset)
	return nil
}

func runSessionShow(cmd *cobra.Command, args []string) error {
	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	sessionID, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid session ID %q: must be a number", args[0])
	}

	rec, err := s.LoadRecording(sessionID)
	if err != nil {
		return fmt.Errorf("loading recording for session #%d: %w", sessionID, err)
	}

	printHeader(fmt.Sprintf("Session Recording #%d", rec.SessionID))

	printField("Agent", rec.Agent)
	printField("Started", rec.StartTime.Format("2006-01-02 15:04:05 UTC"))
	if !rec.EndTime.IsZero() {
		printField("Ended", rec.EndTime.Format("2006-01-02 15:04:05 UTC"))
		d := rec.EndTime.Sub(rec.StartTime)
		printField("Duration", fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60))
	}

	if rec.ExitCode == 0 {
		printFieldColored("Exit Code", "0", colorGreen)
	} else {
		printFieldColored("Exit Code", fmt.Sprintf("%d", rec.ExitCode), colorRed)
	}

	printField("Events", fmt.Sprintf("%d", len(rec.Events)))

	if len(rec.Events) > 0 {
		fmt.Println()
		fmt.Printf("  %sEvent Timeline:%s\n", colorBold, colorReset)
		fmt.Println(colorDim + "  " + strings.Repeat("-", 60) + colorReset)

		for i, ev := range rec.Events {
			typeColor := colorWhite
			switch ev.Type {
			case "stdout":
				typeColor = colorGreen
			case "stderr":
				typeColor = colorRed
			case "stdin":
				typeColor = colorYellow
			case "meta":
				typeColor = colorCyan
			}

			timestamp := ev.Timestamp.Format("15:04:05")
			data := truncate(firstLine(ev.Data), 70)

			fmt.Printf("  %s%s%s %s%-6s%s %s\n",
				colorDim, timestamp, colorReset,
				typeColor, ev.Type, colorReset,
				data)

			// Add a separator every 20 events for readability
			if (i+1)%20 == 0 && i < len(rec.Events)-1 {
				fmt.Printf("  %s... (%d more events) ...%s\n", colorDim, len(rec.Events)-i-1, colorReset)
			}

			// Limit displayed events to avoid flooding the terminal
			if i >= 99 {
				remaining := len(rec.Events) - 100
				if remaining > 0 {
					fmt.Printf("\n  %s... %d more events not shown. Total: %d%s\n",
						colorDim, remaining, len(rec.Events), colorReset)
				}
				break
			}
		}
	}

	fmt.Println()
	return nil
}
