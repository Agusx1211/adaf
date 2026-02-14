package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
)

var spawnWatchCmd = &cobra.Command{
	Use:     "spawn-watch",
	Aliases: []string{"spawn_watch", "spawnwatch"},
	Short:   "Watch spawn output in real-time",
	RunE:    runSpawnWatch,
}

func init() {
	spawnWatchCmd.Flags().Int("spawn-id", 0, "Spawn ID to watch (required)")
	spawnWatchCmd.Flags().Bool("raw", false, "Print raw NDJSON without formatting")
	rootCmd.AddCommand(spawnWatchCmd)
}

func runSpawnWatch(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	raw, _ := cmd.Flags().GetBool("raw")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	var rec *store.SpawnRecord
	for i := 0; i < 50; i++ {
		rec, err = s.GetSpawn(spawnID)
		if err != nil {
			return fmt.Errorf("spawn %d not found: %w", spawnID, err)
		}
		if rec.ChildTurnID > 0 {
			break
		}
		if isTerminalSpawnStatus(rec.Status) {
			fmt.Printf("Spawn #%d is already %s\n", spawnID, rec.Status)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if rec.ChildTurnID == 0 {
		return fmt.Errorf("spawn %d has not started a session yet", spawnID)
	}

	eventsPath := filepath.Join(s.Root(), "records", fmt.Sprintf("%d", rec.ChildTurnID), "events.jsonl")

	var offset int64
	for {
		rec, _ = s.GetSpawn(spawnID)
		terminal := rec != nil && isTerminalSpawnStatus(rec.Status)

		f, err := os.Open(eventsPath)
		if err != nil {
			if os.IsNotExist(err) && !terminal {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			if terminal {
				return nil
			}
			return err
		}

		if offset > 0 {
			f.Seek(offset, 0)
		}

		buf := make([]byte, 64*1024)
		n, readErr := f.Read(buf)
		if n > 0 {
			offset += int64(n)
			chunk := string(buf[:n])
			lines := strings.Split(chunk, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if raw {
					fmt.Println(line)
				} else {
					formatEventLine(line)
				}
			}
		}
		f.Close()

		if readErr != nil && terminal {
			return nil
		}

		if n == 0 {
			if terminal {
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func formatEventLine(line string) {
	var ev store.RecordingEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		fmt.Println(line)
		return
	}

	prefix := fmt.Sprintf("[%s] %s: ", ev.Timestamp.Format("15:04:05"), ev.Type)
	data := ev.Data
	if len(data) > 200 {
		data = data[:200] + "..."
	}
	fmt.Printf("%s%s\n", prefix, data)
}
