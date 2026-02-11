package runtui

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/loop"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

// RunConfig holds everything needed to launch the run TUI.
type RunConfig struct {
	Store       *store.Store
	Agent       agent.Agent
	AgentCfg    agent.Config
	Plan        *store.Plan
	ProjectName string
}

// Run launches the two-column TUI and runs the agent loop concurrently.
func Run(cfg RunConfig) error {
	eventCh := make(chan any, 256)
	streamCh := make(chan stream.RawEvent, 64)

	// Context with signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(cfg.ProjectName, cfg.Plan, cfg.AgentCfg.Name, "", eventCh, cancel)

	p := tea.NewProgram(model, tea.WithAltScreen())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Bridge goroutine: converts stream.RawEvent to AgentEventMsg.
	go func() {
		for ev := range streamCh {
			if ev.Err != nil {
				continue
			}
			eventCh <- AgentEventMsg{Event: ev.Parsed, Raw: ev.Raw}
		}
	}()

	// Agent goroutine: runs the loop and sends lifecycle events.
	go func() {
		// Set up agent config for TUI mode.
		agentCfg := cfg.AgentCfg
		agentCfg.EventSink = streamCh

		// Suppress direct stdout/stderr from the agent process.
		agentCfg.Stdout = io.Discard
		agentCfg.Stderr = io.Discard

		l := &loop.Loop{
			Store:  cfg.Store,
			Agent:  cfg.Agent,
			Config: agentCfg,
			OnStart: func(sessionID int) {
				eventCh <- AgentStartedMsg{SessionID: sessionID}
			},
			OnEnd: func(sessionID int, result *agent.Result) {
				eventCh <- AgentFinishedMsg{
					SessionID: sessionID,
					Result:    result,
				}
			},
		}

		err := l.Run(ctx)
		close(streamCh) // signals bridge to stop
		eventCh <- AgentLoopDoneMsg{Err: err}
		close(eventCh)
	}()

	_, err := p.Run()
	cancel() // ensure agent stops if TUI exits first
	return err
}
