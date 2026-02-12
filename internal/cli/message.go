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

	spawnID, _, err := getTurnContext()
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

// --- adaf spawn-message ---

var spawnMessageCmd = &cobra.Command{
	Use:     "spawn-message [message]",
	Aliases: []string{"spawn_message", "spawnmessage", "send-message", "send_message"},
	Short:   "Send an async message to a child agent",
	Args:    cobra.ExactArgs(1),
	RunE:    runSpawnMessage,
}

func init() {
	spawnMessageCmd.Flags().Int("spawn-id", 0, "Spawn ID (required)")
	spawnMessageCmd.Flags().Bool("interrupt", false, "Interrupt child's current turn")
	rootCmd.AddCommand(spawnMessageCmd)
}

func runSpawnMessage(cmd *cobra.Command, args []string) error {
	content := args[0]
	spawnID, _ := cmd.Flags().GetInt("spawn-id")
	interrupt, _ := cmd.Flags().GetBool("interrupt")
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
		Interrupt: interrupt,
	}
	if err := s.CreateMessage(msg); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	if interrupt {
		// Signal the interrupt so the orchestrator/loop can detect it.
		if err := s.SignalInterrupt(spawnID, content); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to signal interrupt: %v\n", err)
		}
		fmt.Printf("Interrupt message sent to spawn #%d\n", spawnID)
	} else {
		fmt.Printf("Message sent to spawn #%d\n", spawnID)
	}
	return nil
}

// --- adaf spawn-read-messages ---

var spawnReadMessagesCmd = &cobra.Command{
	Use:     "spawn-read-messages",
	Aliases: []string{"spawn_read_messages", "spawnreadmessages", "read-messages", "read_messages"},
	Short:   "Read unread messages from parent",
	RunE:    runSpawnReadMessages,
}

func init() {
	rootCmd.AddCommand(spawnReadMessagesCmd)
}

func runSpawnReadMessages(cmd *cobra.Command, args []string) error {
	spawnID, _, err := getTurnContext()
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

// --- adaf wait-for-spawns ---

var waitForSpawnsCmd = &cobra.Command{
	Use:     "wait-for-spawns",
	Aliases: []string{"wait_for_spawns", "waitforspawns"},
	Short:   "Signal that you want to wait for all spawns to complete",
	Long: `Signal the loop controller that this agent wants to pause and resume
when all spawned sub-agents complete. The agent should exit/finish its
current turn after calling this command. When spawns complete, the loop
will start a new turn with spawn results in the prompt.

This saves API costs by not keeping the agent running while waiting.`,
	RunE: runWaitForSpawns,
}

func init() {
	rootCmd.AddCommand(waitForSpawnsCmd)
}

func runWaitForSpawns(cmd *cobra.Command, args []string) error {
	turnID, _, err := getTurnContext()
	if err != nil {
		return err
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	if err := s.SignalWait(turnID); err != nil {
		return fmt.Errorf("signaling wait: %w", err)
	}

	fmt.Println("Wait signal created. Finish your current turn — the loop will resume when all spawns complete.")
	return nil
}

// --- adaf parent-notify ---

var parentNotifyCmd = &cobra.Command{
	Use:     "parent-notify [message]",
	Aliases: []string{"parent_notify", "parentnotify", "notify-parent", "notify_parent"},
	Short:   "Send a non-blocking notification to parent agent",
	Long: `Send a notification message to the parent agent without blocking.
Unlike parent-ask, this does not wait for a reply.`,
	Args: cobra.ExactArgs(1),
	RunE: runParentNotify,
}

func init() {
	rootCmd.AddCommand(parentNotifyCmd)
}

func runParentNotify(cmd *cobra.Command, args []string) error {
	content := args[0]

	spawnID, _, err := getTurnContext()
	if err != nil {
		return err
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	msg := &store.SpawnMessage{
		SpawnID:   spawnID,
		Direction: "child_to_parent",
		Type:      "notify",
		Content:   content,
	}
	if err := s.CreateMessage(msg); err != nil {
		return fmt.Errorf("sending notification: %w", err)
	}

	fmt.Println("Notification sent to parent.")
	return nil
}
