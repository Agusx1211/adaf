package agent

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stream"
)

// ClaudeAgent runs Anthropic's claude CLI tool.
type ClaudeAgent struct{}

// NewClaudeAgent creates a new ClaudeAgent.
func NewClaudeAgent() *ClaudeAgent {
	return &ClaudeAgent{}
}

// Name returns "claude".
func (c *ClaudeAgent) Name() string {
	return "claude"
}

// Run executes the claude CLI with the given configuration.
//
// It uses --print (-p) to enable non-interactive mode and passes the prompt
// via stdin. Additional flags such as --model and
// --dangerously-skip-permissions can be supplied via cfg.Args.
//
// Output is streamed in real-time using --output-format stream-json --verbose,
// which produces NDJSON events that are parsed, displayed, and recorded.
// The --verbose flag is required when using --output-format stream-json.
func (c *ClaudeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "claude"
	}

	// Build arguments: start with configured defaults, then append streaming
	// flags.
	args := make([]string, 0, len(cfg.Args)+5)
	args = append(args, cfg.Args...)

	// --print (-p) enables non-interactive mode (print response and exit).
	// --output-format stream-json produces NDJSON events on stdout.
	// --verbose is required by the CLI when using stream-json output format.
	args = append(args, "--print", "--output-format", "stream-json", "--verbose")

	// Resume a previous session if a session ID is provided.
	if cfg.ResumeSessionID != "" {
		args = append(args, "--resume", cfg.ResumeSessionID)
	}

	debug.LogKV("agent.claude", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
		"resume_session", cfg.ResumeSessionID,
		"has_event_sink", cfg.EventSink != nil,
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Use shared helpers for common setup.
	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	setupEnv(cmd, cfg.Env)
	setupStdin(cmd, cfg.Prompt, recorder)

	return runStreamAgent(ctx, cmd, cfg, recorder, "claude", cmdName, args, stream.Parse)
}
