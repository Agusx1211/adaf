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
// We explicitly pass --output text to ensure deterministic plain-text
// output. Vibe also supports --output json (all messages at end) and
// --output streaming (NDJSON per message), but we use text mode to keep
// the buffer-agent pattern simple.
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
		// Request explicit text output so we get clean human-readable
		// output rather than depending on vibe's default, which could
		// change across versions.
		args = append(args, "--output", "text")
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
	cmd.Stdout = io.MultiWriter(&stdoutBuf, recorder.WrapWriter(stdoutW, "stdout"))
	cmd.Stderr = io.MultiWriter(&stderrBuf, recorder.WrapWriter(stderrW, "stderr"))

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
