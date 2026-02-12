package agent

import (
	"context"
	"io"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stream"
)

// Config holds the configuration for running an agent.
type Config struct {
	Name     string            // agent name: "claude", "codex", "vibe", or custom
	Command  string            // path to the CLI binary
	Args     []string          // default arguments appended to every invocation
	WorkDir  string            // target repository directory (cwd for the process)
	Env      map[string]string // extra environment variables
	Prompt   string            // the prompt/message to send (piped to stdin or passed as arg)
	MaxTurns int               // max loop iterations (0 = infinite)
	TurnID   int               // current turn ID for recording

	// ResumeSessionID is the session/thread ID from a previous agent run.
	// When set, the agent should resume the previous session instead of
	// starting a new one (e.g. --resume for Claude/Gemini, exec resume for Codex).
	ResumeSessionID string

	// EventSink, when set, receives parsed Claude stream events instead of
	// displaying them to stdout. Used by the TUI to render events in its
	// own viewport.
	EventSink chan<- stream.RawEvent

	// Stdout and Stderr override the default os.Stdout/os.Stderr for agent
	// process output. Nil means use the OS defaults.
	Stdout io.Writer
	Stderr io.Writer
}

// Result holds the outcome of a single agent run.
type Result struct {
	ExitCode       int
	Duration       time.Duration
	Output         string // captured stdout
	Error          string // captured stderr
	AgentSessionID string // session/thread ID captured from stream init event
}

// Agent is the interface that all agent runners must implement.
type Agent interface {
	// Name returns the human-readable name of this agent (e.g. "claude", "codex").
	Name() string

	// Run executes the agent with the given configuration and records I/O
	// through the provided recorder. It blocks until the agent process exits
	// or the context is cancelled.
	Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error)
}
