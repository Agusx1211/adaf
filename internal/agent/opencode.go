package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/recording"
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
// It uses the "run" subcommand in non-interactive mode. Prompt text is passed
// via stdin to avoid argv size limits on long prompts.
//
// The SST fork (actively maintained, installed via npm) uses:
//
//	opencode run [message..] [--model provider/model] [--format json]
//
// The archived Go version used:
//
//	opencode -p "prompt" [-f json]
//
// We target the SST fork since that is what users install today.
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
	args := make([]string, 0, len(cfg.Args)+4)
	args = append(args, "run")
	args = append(args, cfg.Args...)

	var stdinReader io.Reader
	if cfg.Prompt != "" {
		stdinReader = strings.NewReader(cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	debug.LogKV("agent.opencode", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir
	if stdinReader != nil {
		cmd.Stdin = stdinReader
	}

	// Start the process in its own process group. OpenCode is distributed
	// as a Node.js shim that spawns a compiled Bun binary, which in turn
	// may spawn MCP servers and other children. Without process group
	// kill, cancellation would only kill the shim, leaving the real
	// binary and its children running.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second

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

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutWriters := []io.Writer{
		&stdoutBuf,
		recorder.WrapWriter(stdoutW, "stdout"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, ""); w != nil {
		stdoutWriters = append(stdoutWriters, w)
	}
	cmd.Stdout = io.MultiWriter(stdoutWriters...)

	stderrWriters := []io.Writer{
		&stderrBuf,
		recorder.WrapWriter(stderrW, "stderr"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, "[stderr] "); w != nil {
		stderrWriters = append(stderrWriters, w)
	}
	cmd.Stderr = io.MultiWriter(stderrWriters...)

	recorder.RecordMeta("agent", "opencode")
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	debug.LogKV("agent.opencode", "process starting", "binary", cmdName)
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			debug.LogKV("agent.opencode", "cmd.Run() error (not ExitError)", "error", err)
			return nil, fmt.Errorf("opencode agent: failed to run command: %w", err)
		}
	}
	debug.LogKV("agent.opencode", "process finished",
		"exit_code", exitCode,
		"duration", duration,
		"output_len", stdoutBuf.Len(),
	)

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   stdoutBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
