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
	"github.com/agusx1211/adaf/internal/store"
)

var attachCmd = &cobra.Command{
	Use:     "attach [session-id]",
	Aliases: []string{"reattach", "connect"},
	Short:   "Attach to a running session",
	Long: `Reattach to a running adaf session. The session's event history is replayed
and then live events are streamed in real-time.

Press Ctrl+D to detach (session continues running).
Press q to detach (session continues running).
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
	defer signal.Stop(sigCh)
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
	runModel := runtui.NewModel(
		meta.ProjectName,
		nil, // no plan in attach mode
		meta.AgentName,
		"",
		eventCh,
		cancelFunc,
	)
	if meta.ProjectDir != "" {
		if st, err := store.New(meta.ProjectDir); err == nil {
			runModel.SetStore(st)
		}
	}
	runModel.SetSessionMode(meta.ID)
	loopName := meta.LoopName
	loopSteps := meta.LoopSteps
	if loopName == "" && client.Meta.LoopName != "" {
		loopName = client.Meta.LoopName
		loopSteps = client.Meta.LoopSteps
	}
	if loopName != "" {
		runModel.SetLoopInfo(loopName, loopSteps)
	}
	model := newAttachModel(runModel)

	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	detached := false
	if fm, ok := finalModel.(attachModel); ok {
		detached = fm.detached
	}

	if detached {
		// Detached — close connection, agent keeps running.
		client.Close()
		fmt.Printf("\n  %sDetached from session #%d. Use 'adaf attach %d' to reattach.%s\n",
			colorDim, meta.ID, meta.ID, colorReset)
		return err
	}

	// Handle non-detach exits.
	select {
	case <-ctx.Done():
		// Cancelled — agent was stopped.
	default:
		client.Close()
	}

	return err
}

type attachModel struct {
	inner    runtui.Model
	detached bool
}

func newAttachModel(inner runtui.Model) attachModel {
	return attachModel{inner: inner}
}

func (m attachModel) Init() tea.Cmd {
	return m.inner.Init()
}

func (m attachModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		if !m.inner.Done() && (key == "ctrl+d" || key == "q") {
			m.detached = true
			return m, tea.Quit
		}

	case runtui.DetachMsg:
		m.detached = true
		return m, tea.Quit
	}

	updated, cmd := m.inner.Update(msg)
	if inner, ok := updated.(runtui.Model); ok {
		m.inner = inner
	}
	return m, cmd
}

func (m attachModel) View() string {
	return m.inner.View()
}
