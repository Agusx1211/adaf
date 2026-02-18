package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

// --- adaf parent-ask ---

var parentAskCmd = &cobra.Command{
	Use:     "parent-ask [question]",
	Aliases: []string{"parent_ask", "parentask", "ask-parent", "ask_parent"},
	Short:   "Ask parent agent a question (blocks until answered)",
	Args:    cobra.ExactArgs(1),
	RunE:    runParentAsk,
}

func init() {
	parentAskCmd.Flags().Duration("timeout", 10*time.Minute, "Timeout waiting for reply")
	rootCmd.AddCommand(parentAskCmd)
}

func runParentAsk(cmd *cobra.Command, args []string) error {
	question := args[0]
	timeout, _ := cmd.Flags().GetDuration("timeout")

	spawnID, _, _, err := getTurnContext()
	if err != nil {
		return err
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	// Check for existing pending ask.
	existing, err := s.PendingAsk(spawnID)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("there is already a pending question (message #%d): %s", existing.ID, truncate(existing.Content, 80))
	}

	// Create the ask message.
	msg := &store.SpawnMessage{
		SpawnID:   spawnID,
		Direction: "child_to_parent",
		Type:      "ask",
		Content:   question,
	}
	if err := s.CreateMessage(msg); err != nil {
		return fmt.Errorf("creating ask message: %w", err)
	}

	// Update spawn status to awaiting_input.
	rec, err := s.GetSpawn(spawnID)
	if err == nil {
		rec.Status = "awaiting_input"
		s.UpdateSpawn(rec)
	}

	// Poll for reply.
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if spawn was completed/failed externally.
		rec, err = s.GetSpawn(spawnID)
		if err == nil && isTerminalSpawnStatus(rec.Status) {
			return fmt.Errorf("spawn was terminated while waiting for reply (status=%s)", rec.Status)
		}

		// Check for reply.
		msgs, err := s.ListMessages(spawnID)
		if err == nil {
			for _, m := range msgs {
				if m.Type == "reply" && m.ReplyToID == msg.ID {
					fmt.Println(m.Content)
					return nil
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timed out waiting for reply after %s", timeout)
}

// --- adaf spawn-reply ---

var spawnReplyCmd = &cobra.Command{
	Use:     "spawn-reply [answer]",
	Aliases: []string{"spawn_reply", "spawnreply", "reply"},
	Short:   "Reply to a child agent's question",
	Args:    cobra.ExactArgs(1),
	RunE:    runSpawnReply,
}

func init() {
	spawnReplyCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	rootCmd.AddCommand(spawnReplyCmd)
}

func runSpawnReply(cmd *cobra.Command, args []string) error {
	answer := args[0]
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	// Find pending ask.
	ask, err := s.PendingAsk(spawnID)
	if err != nil {
		return err
	}
	if ask == nil {
		return fmt.Errorf("no pending question from spawn #%d", spawnID)
	}

	// Create reply message.
	reply := &store.SpawnMessage{
		SpawnID:   spawnID,
		Direction: "parent_to_child",
		Type:      "reply",
		Content:   answer,
		ReplyToID: ask.ID,
	}
	if err := s.CreateMessage(reply); err != nil {
		return fmt.Errorf("creating reply: %w", err)
	}

	// Update spawn status back to running.
	rec, err := s.GetSpawn(spawnID)
	if err == nil && rec.Status == "awaiting_input" {
		rec.Status = "running"
		s.UpdateSpawn(rec)
	}

	fmt.Printf("Replied to spawn #%d question: %s\n", spawnID, truncate(ask.Content, 80))
	return nil
}

// --- adaf wait-for-spawns ---

var waitForSpawnsCmd = &cobra.Command{
	Use:     "wait-for-spawns",
	Aliases: []string{"wait_for_spawns", "waitforspawns"},
	Short:   "Signal that you want to wait for spawns with periodic review checkpoints",
	Long: `Signal the loop controller that this agent wants to pause and resume
while spawned sub-agents run. The agent should exit/finish its current
turn after calling this command. The loop resumes the same turn when
spawns complete, and may also resume periodically with running-spawn
review checkpoints so you can inspect health and intervene if needed.

This saves API costs by not keeping the agent running while waiting.
After calling this command, stop immediately and run no further commands.`,
	RunE: runWaitForSpawns,
}

func init() {
	// Accept --timeout for compatibility (the command is a signal, not a wait;
	// the actual timeout is managed by the loop controller).
	waitForSpawnsCmd.Flags().Int("timeout", 0, "Accepted for compatibility (ignored)")
	waitForSpawnsCmd.Flags().MarkHidden("timeout")
	rootCmd.AddCommand(waitForSpawnsCmd)
}

func runWaitForSpawns(cmd *cobra.Command, args []string) error {
	turnID, _, _, err := getTurnContext()
	if err != nil {
		return err
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	signaled := false
	if daemonSessionID, ok := currentDaemonSessionID(); ok {
		resp, reqErr := session.RequestWait(daemonSessionID, turnID)
		if reqErr == nil && resp != nil && resp.OK {
			signaled = true
		} else if reqErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: daemon wait signal failed; falling back to file signal: %v\n", reqErr)
		} else if resp != nil && strings.TrimSpace(resp.Error) != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: daemon wait signal rejected; falling back to file signal: %s\n", resp.Error)
		}
	}
	if !signaled {
		if err := s.SignalWait(turnID); err != nil {
			return fmt.Errorf("signaling wait: %w", err)
		}
	}

	fmt.Println("Wait signal created. Stop now and end this turn — the loop will resume on spawn completion or a periodic review checkpoint.")
	return nil
}
