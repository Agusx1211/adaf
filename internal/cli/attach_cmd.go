package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/session"
)

var attachCmd = &cobra.Command{
	Use:     "attach [session-id]",
	Aliases: []string{"reattach", "connect"},
	Short:   "Attach to a running session",
	Long: `Reattach to a running adaf session. The session's event history is replayed
and then live events are streamed in real-time.

Press Ctrl+D to detach (session continues running).
Press Ctrl+C to stop the agent and detach.

Use 'adaf sessions' to list available sessions.`,
	Args: cobra.ExactArgs(1),
	RunE: runAttach,
}

func init() {
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("session management is not available inside an agent context")
	}

	if !isatty.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("attach requires an interactive terminal")
	}

	// Find the session.
	meta, err := session.FindSessionByPartial(args[0])
	if err != nil {
		return err
	}

	if meta.Status != "running" && meta.Status != "starting" {
		return fmt.Errorf("session %d is not running (status: %s)", meta.ID, meta.Status)
	}

	// Connect to the session daemon.
	client, err := session.ConnectToSession(meta.ID)
	if err != nil {
		return fmt.Errorf("connecting to session %d: %w", meta.ID, err)
	}

	fmt.Printf("  %sAttaching to session #%d (%s)...%s\n", colorDim, meta.ID, meta.ProfileName, colorReset)

	// Set up the event channel and TUI.
	eventCh := make(chan any, 256)

	// Create a cancel function that sends cancel to the daemon.
	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc := func() {
		client.Cancel()
		cancel()
	}

	// Handle signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancelFunc()
	}()

	// Stream events from the daemon to the event channel.
	go func() {
		err := client.StreamEvents(eventCh, nil)
		if err != nil {
			// Connection error; eventCh is already closed by StreamEvents.
			_ = err
		}
	}()

	// Create the TUI model.
	model := runtui.NewModel(
		meta.ProjectName,
		nil, // no plan in attach mode
		meta.AgentName,
		"",
		eventCh,
		cancelFunc,
	)
	model.SetSessionMode(meta.ID)

	p := tea.NewProgram(model, tea.WithAltScreen())

	_, err = p.Run()

	// Check if we detached (TUI exited without cancelling).
	select {
	case <-ctx.Done():
		// Cancelled — agent was stopped.
	default:
		// Detached — just close connection, agent keeps running.
		client.Close()
		fmt.Printf("\n  %sDetached from session #%d. Use 'adaf attach %d' to reattach.%s\n",
			colorDim, meta.ID, meta.ID, colorReset)
	}

	return err
}
