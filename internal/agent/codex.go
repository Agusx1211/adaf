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

// CodexAgent runs OpenAI's codex CLI tool.
type CodexAgent struct{}

// NewCodexAgent creates a new CodexAgent.
func NewCodexAgent() *CodexAgent {
	return &CodexAgent{}
}

// Name returns "codex".
func (c *CodexAgent) Name() string {
	return "codex"
}

// Run executes the codex CLI with the given configuration.
//
// The prompt is passed as a positional argument to codex. Additional flags
// (e.g. --model, --approval-mode) can be supplied via cfg.Args.
func (c *CodexAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "codex"
	}

	// Build arguments: defaults first, then the prompt as a positional arg.
	args := make([]string, 0, len(cfg.Args)+1)
	args = append(args, cfg.Args...)

	if cfg.Prompt != "" {
		args = append(args, cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Environment: inherit + overlay.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Capture output.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdoutBuf, recorder.WrapWriter(os.Stdout, "stdout"))
	cmd.Stderr = io.MultiWriter(&stderrBuf, recorder.WrapWriter(os.Stderr, "stderr"))

	recorder.RecordMeta("agent", "codex")
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
			return nil, fmt.Errorf("codex agent: failed to run command: %w", err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   stdoutBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
