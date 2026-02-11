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

	stdoutWriter := io.MultiWriter(&stdoutBuf, recorder.WrapWriter(os.Stdout, "stdout"))
	stderrWriter := io.MultiWriter(&stderrBuf, recorder.WrapWriter(os.Stderr, "stderr"))

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

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
			// If we can't determine exit code (e.g. command not found), return the error.
			return nil, fmt.Errorf("agent %q: failed to run command: %w", g.name, err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   stdoutBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
