package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
)

var treeCmd = &cobra.Command{
	Use:     "tree",
	Aliases: []string{"hierarchy", "spawn-tree", "spawn_tree"},
	Short: "Show agent hierarchy tree",
	Long: `Display the hierarchical tree of spawned sub-agents.

Shows parent-child relationships, status, elapsed time, and task descriptions
for all active spawns. Use --all to include completed/rejected spawns and
--watch for a live-updating view.

Examples:
  adaf tree                               # Show active spawns
  adaf tree --all                         # Include completed spawns
  adaf tree --watch                       # Live refresh every 2s`,
	RunE: runTree,
}

func init() {
	treeCmd.Flags().Bool("all", false, "Include completed/rejected/merged spawns")
	treeCmd.Flags().Bool("watch", false, "Refresh every 2 seconds")
	rootCmd.AddCommand(treeCmd)
}

func runTree(cmd *cobra.Command, args []string) error {
	showAll, _ := cmd.Flags().GetBool("all")
	watch, _ := cmd.Flags().GetBool("watch")

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	for {
		if watch {
			// Clear screen.
			fmt.Print("\033[2J\033[H")
		}

		if err := printTree(s, showAll); err != nil {
			return err
		}

		if !watch {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}

func printTree(s *store.Store, showAll bool) error {
	records, err := s.ListSpawns()
	if err != nil {
		return err
	}

	if len(records) == 0 {
		fmt.Println("No spawns found.")
		return nil
	}

	fmt.Println(styleBoldCyan + "Agent Tree" + colorReset)
	fmt.Println(colorDim + "──────────" + colorReset)

	// Build parent -> children map.
	children := make(map[int][]store.SpawnRecord)
	var roots []store.SpawnRecord

	// All parent session IDs that are themselves spawn child sessions.
	childSessionToSpawn := make(map[int]int) // child_session_id -> spawn_id
	for _, r := range records {
		if r.ChildSessionID > 0 {
			childSessionToSpawn[r.ChildSessionID] = r.ID
		}
	}

	for _, r := range records {
		if !showAll && isTerminalStatus(r.Status) {
			continue
		}
		// Check if this spawn's parent is itself a spawn child.
		if _, ok := childSessionToSpawn[r.ParentSessionID]; ok {
			// This is a nested spawn — group under parent spawn's child session.
			parentSpawnID := childSessionToSpawn[r.ParentSessionID]
			children[parentSpawnID] = append(children[parentSpawnID], r)
		} else {
			roots = append(roots, r)
		}
	}

	for _, r := range roots {
		printTreeNode(s, r, children, "")
	}

	return nil
}

func printTreeNode(s *store.Store, r store.SpawnRecord, children map[int][]store.SpawnRecord, indent string) {
	statusStr := coloredStatus(r.Status)
	elapsed := elapsedStr(r)
	task := truncate(r.Task, 60)

	fmt.Printf("%s[%d] %s (%s) - \"%s\" [%s]\n", indent, r.ID, r.ChildProfile, statusStr, task, elapsed)

	// Show pending question for awaiting_input.
	if r.Status == "awaiting_input" {
		ask, err := s.PendingAsk(r.ID)
		if err == nil && ask != nil {
			fmt.Printf("%s    %sQuestion: \"%s\"%s\n", indent, colorYellow, truncate(ask.Content, 80), colorReset)
		}
	}

	// Print children.
	kids := children[r.ID]
	for _, child := range kids {
		printTreeNode(s, child, children, indent+"  ")
	}
}

func coloredStatus(status string) string {
	switch status {
	case "running":
		return colorYellow + status + colorReset
	case "awaiting_input":
		return styleBoldBlue + status + colorReset
	case "completed":
		return colorGreen + status + colorReset
	case "merged":
		return colorDim + colorGreen + status + colorReset
	case "failed":
		return colorRed + status + colorReset
	case "rejected":
		return colorRed + status + colorReset
	default:
		return status
	}
}

func elapsedStr(r store.SpawnRecord) string {
	if !r.CompletedAt.IsZero() {
		return r.CompletedAt.Sub(r.StartedAt).Round(time.Second).String()
	}
	if !r.StartedAt.IsZero() {
		return time.Since(r.StartedAt).Round(time.Second).String()
	}
	return "0s"
}

func isTerminalStatus(status string) bool {
	return status == "completed" || status == "failed" || status == "merged" || status == "rejected"
}
