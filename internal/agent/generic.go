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

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/recording"
)

// GenericAgent wraps any CLI tool as an agent. It executes the configured
// command, pipes the prompt to stdin, and captures stdout/stderr through
// the recorder.
type GenericAgent struct {
	name string
}

// NewGenericAgent creates a new GenericAgent with the given name.
func NewGenericAgent(name string) *GenericAgent {
	return &GenericAgent{name: name}
}

// Name returns the agent name.
func (g *GenericAgent) Name() string {
	return g.name
}

// Run executes the configured command, streaming output through the recorder.
func (g *GenericAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		return nil, fmt.Errorf("agent %q: no command configured", g.name)
	}

	args := make([]string, len(cfg.Args))
	copy(args, cfg.Args)

	debug.LogKV("agent.generic", "building command",
		"name", g.name,
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Build environment: inherit current env, then overlay extras.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// If a prompt is provided, pipe it to stdin.
	if cfg.Prompt != "" {
		cmd.Stdin = strings.NewReader(cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	// Capture stdout and stderr, streaming to both the recorder and
	// in-memory buffers so we can return the full output in Result.
	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutW := cfg.Stdout
	if stdoutW == nil {
		stdoutW = os.Stdout
	}
	stderrW := cfg.Stderr
	if stderrW == nil {
		stderrW = os.Stderr
	}

	stdoutWriters := []io.Writer{
		&stdoutBuf,
		recorder.WrapWriter(stdoutW, "stdout"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, ""); w != nil {
		stdoutWriters = append(stdoutWriters, w)
	}

	stderrWriters := []io.Writer{
		&stderrBuf,
		recorder.WrapWriter(stderrW, "stderr"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, "[stderr] "); w != nil {
		stderrWriters = append(stderrWriters, w)
	}

	cmd.Stdout = io.MultiWriter(stdoutWriters...)
	cmd.Stderr = io.MultiWriter(stderrWriters...)

	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	debug.LogKV("agent.generic", "process starting", "name", g.name, "binary", cmdName)
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			debug.LogKV("agent.generic", "cmd.Run() error (not ExitError)", "name", g.name, "error", err)
			return nil, fmt.Errorf("agent %q: failed to run command: %w", g.name, err)
		}
	}
	debug.LogKV("agent.generic", "process finished",
		"name", g.name,
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
