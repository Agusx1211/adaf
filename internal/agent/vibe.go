package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
)

// VibeAgent runs the vibe CLI tool.
type VibeAgent struct{}

// NewVibeAgent creates a new VibeAgent.
func NewVibeAgent() *VibeAgent {
	return &VibeAgent{}
}

// Name returns "vibe".
func (v *VibeAgent) Name() string {
	return "vibe"
}

// Run executes the vibe CLI with the given configuration.
//
// The prompt is passed via the -p flag which activates programmatic mode.
// In programmatic mode vibe auto-approves all tool executions and exits
// after completing the response (equivalent to using the "auto-approve"
// agent profile).
//
// We select output mode based on runtime:
//   - TUI/EventSink mode: --output streaming for realtime NDJSON updates.
//   - Non-TUI mode: --output text for deterministic plain-text output.
//
// Model selection is done via the VIBE_ACTIVE_MODEL environment variable
// (set in cfg.Env) rather than a --model flag, because vibe uses
// pydantic-settings with env_prefix="VIBE_" to override any config field.
//
// Additional flags (e.g. --max-turns, --max-price) can be supplied via
// cfg.Args.
func (v *VibeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "vibe"
	}

	// Build arguments: start with configured defaults, then append
	// programmatic-mode flags and the prompt.
	args := make([]string, 0, len(cfg.Args)+4)
	args = append(args, cfg.Args...)

	if cfg.Prompt != "" {
		args = append(args, "-p", cfg.Prompt)
		outputMode := "text"
		if cfg.EventSink != nil {
			outputMode = "streaming"
		}
		// Request explicit output mode rather than depending on defaults
		// that may change across vibe releases.
		args = append(args, "--output", outputMode)
		recorder.RecordStdin(cfg.Prompt)
	}

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Environment: inherit + overlay.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Determine stdout/stderr writers, respecting cfg overrides.
	stdoutW := cfg.Stdout
	if stdoutW == nil {
		stdoutW = os.Stdout
	}
	stderrW := cfg.Stderr
	if stderrW == nil {
		stderrW = os.Stderr
	}

	// Capture output.
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutWriters := []io.Writer{
		&stdoutBuf,
		recorder.WrapWriter(stdoutW, "stdout"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.SessionID, ""); w != nil {
		stdoutWriters = append(stdoutWriters, w)
	}
	cmd.Stdout = io.MultiWriter(stdoutWriters...)

	stderrWriters := []io.Writer{
		&stderrBuf,
		recorder.WrapWriter(stderrW, "stderr"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.SessionID, "[stderr] "); w != nil {
		stderrWriters = append(stderrWriters, w)
	}
	cmd.Stderr = io.MultiWriter(stderrWriters...)

	recorder.RecordMeta("agent", "vibe")
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("vibe agent: failed to run command: %w", err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   stdoutBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
