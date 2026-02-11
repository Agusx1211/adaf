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
// The prompt is piped to stdin. Additional flags can be supplied via cfg.Args.
func (v *VibeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "vibe"
	}

	args := make([]string, len(cfg.Args))
	copy(args, cfg.Args)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Environment: inherit + overlay.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Pipe prompt to stdin.
	if cfg.Prompt != "" {
		cmd.Stdin = strings.NewReader(cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	// Capture output.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdoutBuf, recorder.WrapWriter(os.Stdout, "stdout"))
	cmd.Stderr = io.MultiWriter(&stderrBuf, recorder.WrapWriter(os.Stderr, "stderr"))

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
