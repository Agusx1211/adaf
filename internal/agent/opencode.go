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

// OpencodeAgent runs the opencode CLI tool.
type OpencodeAgent struct{}

// NewOpencodeAgent creates a new OpencodeAgent.
func NewOpencodeAgent() *OpencodeAgent {
	return &OpencodeAgent{}
}

// Name returns "opencode".
func (o *OpencodeAgent) Name() string {
	return "opencode"
}

// Run executes the opencode CLI with the given configuration.
//
// It uses the "run" subcommand in non-interactive mode with --format json to
// get structured NDJSON output. Prompt text is passed via stdin to avoid argv
// size limits on long prompts.
//
// The SST fork (actively maintained, installed via npm) uses:
//
//	opencode run [message..] [--model provider/model] [--format json]
//	opencode run --session <id> [message..] [--format json]
//
// The archived Go version used:
//
//	opencode -p "prompt" [-f json]
//
// We target the SST fork since that is what users install today.
//
// Session resume is supported via --session <id>. The session ID is captured
// from the sessionID field present on every NDJSON event.
//
// OpenCode is a Bun-compiled native binary distributed via npm. The npm
// package includes a Node.js shim that finds and spawns the platform-
// specific binary. Because of this two-layer process tree, we set Setpgid
// and kill the entire process group on cancellation to avoid orphans.
func (o *OpencodeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "opencode"
	}

	// Build arguments: "run" subcommand, then configured flags.
	args := make([]string, 0, len(cfg.Args)+6)
	args = append(args, "run")
	args = append(args, cfg.Args...)

	// Request structured NDJSON output for stream parsing.
	if !hasFlag(cfg.Args, "--format") {
		args = append(args, "--format", "json")
	}

	// Resume a previous session if a session ID is provided.
	if cfg.ResumeSessionID != "" {
		args = append(args, "--session", cfg.ResumeSessionID)
	}

	debug.LogKV("agent.opencode", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
		"resume_session", cfg.ResumeSessionID,
		"has_event_sink", cfg.EventSink != nil,
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Start the process in its own process group. OpenCode is distributed
	// as a Node.js shim that spawns a compiled Bun binary, which in turn
	// may spawn MCP servers and other children. Without process group
	// kill, cancellation would only kill the shim, leaving the real
	// binary and its children running.
	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second

	setupEnv(cmd, cfg.Env)
	setupStdin(cmd, cfg.Prompt, recorder)

	// Set up stdout pipe for streaming NDJSON parsing.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opencode agent: stdout pipe: %w", err)
	}

	ss := setupStreamStderr(cmd, cfg, recorder)
	recordMeta(recorder, "opencode", cmdName, args, cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		debug.LogKV("agent.opencode", "process start failed", "error", err)
		return nil, fmt.Errorf("opencode agent: failed to start command: %w", err)
	}
	debug.LogKV("agent.opencode", "process started", "pid", cmd.Process.Pid)

	// Parse the NDJSON stream.
	events := stream.ParseOpencode(ctx, stdoutPipe)

	// Run the stream loop (handles both TUI and legacy display modes).
	text, agentSessionID := runStreamLoop(cfg, events, recorder, start, ss.W)

	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode, err := extractExitCode(waitErr)
	if err != nil {
		debug.LogKV("agent.opencode", "cmd.Wait() error (not ExitError)", "error", err)
		return nil, fmt.Errorf("opencode agent: failed to run command: %w", err)
	}

	debug.LogKV("agent.opencode", "process finished",
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
