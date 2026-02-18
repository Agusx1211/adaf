package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var spawnDiffCmd = &cobra.Command{
	Use:     "spawn-diff",
	Aliases: []string{"spawn_diff", "spawndiff"},
	Short:   "Show diff of a spawn's changes",
	RunE:    runSpawnDiff,
}

func init() {
	spawnDiffCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	spawnDiffCmd.Flags().Bool("name-only", false, "Show only names of changed files")
	spawnDiffCmd.SuggestFor = append(spawnDiffCmd.SuggestFor, "diff")
	rootCmd.AddCommand(spawnDiffCmd)
}

func runSpawnDiff(cmd *cobra.Command, args []string) error {
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	o, err := ensureOrchestrator()
	if err != nil {
		return err
	}

	diff, err := o.Diff(context.Background(), spawnID)
	if err != nil {
		return err
	}

	nameOnly, _ := cmd.Flags().GetBool("name-only")
	if nameOnly {
		files := extractDiffFileNames(diff)
		for _, f := range files {
			fmt.Println(f)
		}
		return nil
	}

	fmt.Print(diff)
	return nil
}

// extractDiffFileNames parses unified diff output for file paths.
func extractDiffFileNames(diff string) []string {
	seen := make(map[string]struct{})
	var names []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			name := strings.TrimPrefix(line, "+++ b/")
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				names = append(names, name)
			}
		} else if strings.HasPrefix(line, "--- a/") {
			name := strings.TrimPrefix(line, "--- a/")
			if name != "/dev/null" {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					names = append(names, name)
				}
			}
		}
	}
	return names
}
