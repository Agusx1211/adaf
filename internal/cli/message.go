package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
)

// --- adaf parent-ask ---

var parentAskCmd = &cobra.Command{
	Use:   "parent-ask [question]",
	Short: "Ask parent agent a question (blocks until answered)",
	Args:  cobra.ExactArgs(1),
	RunE:  runParentAsk,
}

func init() {
	parentAskCmd.Flags().Duration("timeout", 10*time.Minute, "Timeout waiting for reply")
	rootCmd.AddCommand(parentAskCmd)
}

func runParentAsk(cmd *cobra.Command, args []string) error {
	question := args[0]
	timeout, _ := cmd.Flags().GetDuration("timeout")

	spawnID, _, err := getSessionContext()
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
		if err == nil && (rec.Status == "completed" || rec.Status == "failed" || rec.Status == "rejected") {
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
	Use:   "spawn-reply [answer]",
	Short: "Reply to a child agent's question",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpawnReply,
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

// --- adaf spawn-message ---

var spawnMessageCmd = &cobra.Command{
	Use:   "spawn-message [message]",
	Short: "Send an async message to a child agent",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpawnMessage,
}

func init() {
	spawnMessageCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	rootCmd.AddCommand(spawnMessageCmd)
}

func runSpawnMessage(cmd *cobra.Command, args []string) error {
	content := args[0]
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	if spawnID == 0 {
		return fmt.Errorf("--spawn-id is required")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	msg := &store.SpawnMessage{
		SpawnID:   spawnID,
		Direction: "parent_to_child",
		Type:      "message",
		Content:   content,
	}
	if err := s.CreateMessage(msg); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	fmt.Printf("Message sent to spawn #%d\n", spawnID)
	return nil
}

// --- adaf spawn-read-messages ---

var spawnReadMessagesCmd = &cobra.Command{
	Use:   "spawn-read-messages",
	Short: "Read unread messages from parent",
	RunE:  runSpawnReadMessages,
}

func init() {
	rootCmd.AddCommand(spawnReadMessagesCmd)
}

func runSpawnReadMessages(cmd *cobra.Command, args []string) error {
	spawnID, _, err := getSessionContext()
	if err != nil {
		return err
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	msgs, err := s.UnreadMessages(spawnID, "parent_to_child")
	if err != nil {
		return err
	}

	if len(msgs) == 0 {
		fmt.Println("No unread messages.")
		return nil
	}

	var sb strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&sb, "[%s] (%s) %s\n", m.CreatedAt.Format("15:04:05"), m.Type, m.Content)
		s.MarkMessageRead(m.SpawnID, m.ID)
	}
	fmt.Print(sb.String())
	return nil
}
