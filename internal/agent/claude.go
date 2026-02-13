package agent

import (
	"context"
	"fmt"
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
	setupEnv(cmd, cfg.Env)
	setupStdin(cmd, cfg.Prompt, recorder)

	// Set up stdout pipe for streaming NDJSON parsing.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude agent: stdout pipe: %w", err)
	}

	// Set up stderr capture.
	ss := setupStreamStderr(cmd, cfg, recorder)

	// Record metadata.
	recordMeta(recorder, "claude", cmdName, args, cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		debug.LogKV("agent.claude", "process start failed", "error", err)
		return nil, fmt.Errorf("claude agent: failed to start command: %w", err)
	}
	debug.LogKV("agent.claude", "process started", "pid", cmd.Process.Pid)

	// Parse the NDJSON stream.
	events := stream.Parse(ctx, stdoutPipe)

	// Run the stream loop (handles both TUI and legacy display modes).
	text, agentSessionID := runStreamLoop(cfg, events, recorder, start, ss.W)

	waitErr := cmd.Wait()
	duration := time.Since(start)

	// Extract exit code.
	exitCode, err := extractExitCode(waitErr)
	if err != nil {
		debug.LogKV("agent.claude", "cmd.Wait() error (not ExitError)", "error", err)
		return nil, fmt.Errorf("claude agent: failed to run command: %w", err)
	}

	debug.LogKV("agent.claude", "process finished",
		"exit_code", exitCode,
		"duration", duration,
		"output_len", len(text),
		"stderr_len", ss.Buf.Len(),
		"agent_session_id", agentSessionID,
	)

	return &Result{
		ExitCode:       exitCode,
		Duration:       duration,
		Output:         text,
		Error:          ss.Buf.String(),
		AgentSessionID: agentSessionID,
	}, nil
}
