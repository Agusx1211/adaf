package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/orchestrator"
)

var spawnInspectCmd = &cobra.Command{
	Use:     "spawn-inspect",
	Aliases: []string{"spawn_inspect", "spawninspect"},
	Short:   "Inspect a running spawn's recent activity",
	Long: `Show the child agent's recent stream events, formatted for consumption
by a parent agent. Shows the last few tool calls, reasoning blocks, and output.`,
	RunE: runSpawnInspect,
}

func init() {
	spawnInspectCmd.Flags().Int("spawn-id", 0, "Spawn ID to inspect (required)")
	spawnInspectCmd.Flags().Int("last", 20, "Number of recent events to show")
	rootCmd.AddCommand(spawnInspectCmd)
}

func runSpawnInspect(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	last, _ := cmd.Flags().GetInt("last")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	if o := orchestrator.Get(); o != nil {
		events, err := o.InspectSpawn(spawnID)
		if err == nil {
			if len(events) == 0 {
				fmt.Println("No events recorded yet.")
				return nil
			}

			if last > 0 && len(events) > last {
				events = events[len(events)-last:]
			}

			fmt.Printf("Recent activity for spawn #%d (%d events):\n\n", spawnID, len(events))
			for _, ev := range events {
				if ev.Text != "" {
					fmt.Printf("[output] %s\n", truncate(ev.Text, 200))
				} else if ev.Parsed.Type != "" {
					summary := ev.Parsed.Type
					if ev.Parsed.Subtype != "" {
						summary += "/" + ev.Parsed.Subtype
					}
					content := ""
					if ev.Parsed.AssistantMessage != nil {
						for _, cb := range ev.Parsed.AssistantMessage.Content {
							if cb.Text != "" {
								content = truncate(cb.Text, 150)
								break
							}
						}
					}
					if content != "" {
						fmt.Printf("[%s] %s\n", summary, content)
					} else {
						raw := string(ev.Raw)
						if len(raw) > 200 {
							raw = raw[:200] + "..."
						}
						fmt.Printf("[%s] %s\n", summary, raw)
					}
				}
			}
			return nil
		}
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}
	rec, err := s.GetSpawn(spawnID)
	if err != nil {
		return fmt.Errorf("spawn %d not found: %w", spawnID, err)
	}
	if rec.ChildTurnID == 0 {
		fmt.Println("Spawn has not started a child session yet.")
		return nil
	}

	eventsPath := filepath.Join(s.Root(), "records", fmt.Sprintf("%d", rec.ChildTurnID), "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No recorded events yet.")
			return nil
		}
		return fmt.Errorf("reading events: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty = append(nonEmpty, line)
	}
	if len(nonEmpty) == 0 {
		fmt.Println("No recorded events yet.")
		return nil
	}
	if last > 0 && len(nonEmpty) > last {
		nonEmpty = nonEmpty[len(nonEmpty)-last:]
	}

	fmt.Printf("Recent activity for spawn #%d (%d events):\n\n", spawnID, len(nonEmpty))
	for _, line := range nonEmpty {
		formatEventLine(line)
	}
	return nil
}
