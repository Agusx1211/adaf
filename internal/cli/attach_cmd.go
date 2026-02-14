package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

var attachCmd = &cobra.Command{
	Use:     "attach [loop-name|session-id]",
	Aliases: []string{"reattach", "connect"},
	Short:   "Attach to a running loop or session",
	Long: `Reattach to a running adaf loop or session. The TUI loads history from the
project store, then receives a daemon snapshot and live events in real-time.

With no arguments, attaches to the only running session (if exactly one exists).
With a loop name, attaches to the running session for that loop.
With a numeric session ID, attaches to that specific session.

Press Ctrl+D to detach (session continues running).
Press q to detach (session continues running).
Press Ctrl+C to stop the agent and detach.

Use 'adaf sessions' to list available sessions.`,
	Args: cobra.MaximumNArgs(1),
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

	// Find the session to attach to.
	var meta *session.SessionMeta
	var err error

	if len(args) == 0 {
		// No argument: auto-attach to the only running session.
		meta, err = session.FindOnlyRunningSession()
		if err != nil {
			return err
		}
	} else {
		// Try loop name first, then fall back to session ID / profile match.
		meta, err = session.FindRunningByLoopName(args[0])
		if err != nil {
			meta, err = session.FindSessionByPartial(args[0])
			if err != nil {
				return err
			}
		}
	}

	if !session.IsActiveStatus(meta.Status) {
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
		err := client.StreamEvents(eventCh, func() {
			select {
			case eventCh <- runtui.SessionLiveMsg{}:
			default:
			}
		})
		if err != nil {
			// Connection error; eventCh is already closed by StreamEvents.
			_ = err
		}
	}()

	var plan *store.Plan
	var projectStore *store.Store
	if meta.ProjectDir != "" {
		if st, err := store.New(meta.ProjectDir); err == nil {
			projectStore = st
			if cfg, err := session.LoadConfig(meta.ID); err == nil {
				if planID := strings.TrimSpace(cfg.PlanID); planID != "" {
					if p, err := st.GetPlan(planID); err == nil {
						plan = p
					}
				}
			}
			if plan == nil {
				if p, err := st.ActivePlan(); err == nil {
					plan = p
				}
			}
		}
	}

	// Create the TUI model.
	runModel := runtui.NewModel(
		meta.ProjectName,
		plan,
		meta.AgentName,
		"",
		eventCh,
		cancelFunc,
	)
	if projectStore != nil {
		runModel.SetStore(projectStore)
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
		reattachHint := fmt.Sprintf("%d", meta.ID)
		if meta.LoopName != "" {
			reattachHint = meta.LoopName
		}
		fmt.Printf("\n  %sDetached from session #%d. Use 'adaf attach %s' to reattach.%s\n",
			colorDim, meta.ID, reattachHint, colorReset)
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
