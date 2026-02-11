package cli

import (
	"encoding/json"
	"fmt"
	"os"
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

	// List recording directories (records/ and legacy recordings/)
	printHeader("Session Recordings")

	seen := make(map[int]bool)
	var rows [][]string
	for _, recDir := range s.RecordsDirs() {
		entries, err := os.ReadDir(recDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sessionID, err := strconv.Atoi(e.Name())
			if err != nil || seen[sessionID] {
				continue
			}

			rec, err := s.LoadRecording(sessionID)
			if err != nil {
				continue
			}
			seen[sessionID] = true

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
			case "claude_stream":
				typeColor = styleBoldCyan
			}

			timestamp := ev.Timestamp.Format("15:04:05")
			data := truncate(firstLine(ev.Data), 70)

			// For claude_stream events, show a formatted summary instead of raw JSON.
			if ev.Type == "claude_stream" {
				data = summarizeStreamEvent(ev.Data)
			}

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

// summarizeStreamEvent returns a short human-readable summary of a claude_stream
// JSON event, suitable for display in the event timeline.
func summarizeStreamEvent(rawJSON string) string {
	var obj struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype,omitempty"`
		SessionID string `json:"session_id,omitempty"`
		Model     string `json:"model,omitempty"`

		// assistant events
		Message *struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
				Name string `json:"name,omitempty"`
			} `json:"content,omitempty"`
		} `json:"message,omitempty"`

		// content_block_start/delta/stop (streaming mode)
		ContentBlock *struct {
			Type string `json:"type"`
			Name string `json:"name,omitempty"`
		} `json:"content_block,omitempty"`
		Delta *struct {
			Type string `json:"type,omitempty"`
			Text string `json:"text,omitempty"`
		} `json:"delta,omitempty"`

		// result events (top-level fields in Claude stream-json)
		TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
		DurationMS   float64 `json:"duration_ms,omitempty"`
		NumTurns     int     `json:"num_turns,omitempty"`
		ResultText   string  `json:"result,omitempty"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &obj); err != nil {
		return truncate(firstLine(rawJSON), 70)
	}

	switch obj.Type {
	case "system":
		return fmt.Sprintf("[init] session=%s model=%s", obj.SessionID, obj.Model)
	case "assistant":
		if obj.Message != nil {
			var parts []string
			for _, block := range obj.Message.Content {
				switch block.Type {
				case "text":
					text := block.Text
					if len(text) > 50 {
						text = text[:47] + "..."
					}
					parts = append(parts, text)
				case "tool_use":
					parts = append(parts, fmt.Sprintf("[tool:%s]", block.Name))
				case "thinking":
					parts = append(parts, "[thinking]")
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
		return "[assistant]"
	case "content_block_start":
		if obj.ContentBlock != nil {
			switch obj.ContentBlock.Type {
			case "tool_use":
				return fmt.Sprintf("[tool:%s] start", obj.ContentBlock.Name)
			case "thinking":
				return "[thinking] start"
			case "text":
				return "[text] start"
			}
			return fmt.Sprintf("[%s] start", obj.ContentBlock.Type)
		}
	case "content_block_delta":
		if obj.Delta != nil {
			text := obj.Delta.Text
			if len(text) > 60 {
				text = text[:57] + "..."
			}
			switch obj.Delta.Type {
			case "text_delta":
				return fmt.Sprintf("[text] %s", text)
			case "thinking_delta":
				return fmt.Sprintf("[thinking] %s", text)
			case "input_json_delta":
				return "[tool] input delta"
			}
		}
	case "content_block_stop":
		return "[block] stop"
	case "result":
		var parts []string
		if obj.TotalCostUSD > 0 {
			parts = append(parts, fmt.Sprintf("cost=$%.4f", obj.TotalCostUSD))
		}
		if obj.DurationMS > 0 {
			parts = append(parts, fmt.Sprintf("duration=%.1fs", obj.DurationMS/1000))
		}
		if obj.NumTurns > 0 {
			parts = append(parts, fmt.Sprintf("turns=%d", obj.NumTurns))
		}
		if len(parts) > 0 {
			return "[result] " + strings.Join(parts, " ")
		}
		return "[result]"
	}

	return fmt.Sprintf("[%s]", obj.Type)
}
