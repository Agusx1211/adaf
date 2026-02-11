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
// It uses the -p flag to pass the prompt directly as a command-line argument.
// Additional flags such as --model and --dangerously-skip-permissions can be
// supplied via cfg.Args.
//
// The standard output and error streams are captured through the recorder and
// simultaneously forwarded to the real terminal.
func (c *ClaudeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "claude"
	}

	// Build arguments: start with configured defaults, then append -p <prompt>.
	args := make([]string, 0, len(cfg.Args)+2)
	args = append(args, cfg.Args...)

	// Pass the prompt via the -p flag so claude treats it as a non-interactive
	// prompt. If the prompt is empty we still run (claude may read from stdin
	// or use its own interactive mode).
	if cfg.Prompt != "" {
		args = append(args, "-p", cfg.Prompt)
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

	recorder.RecordMeta("agent", "claude")
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
			return nil, fmt.Errorf("claude agent: failed to run command: %w", err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   stdoutBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
