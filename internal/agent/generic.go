package agent

import (
	"context"
	"fmt"
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

	setupEnv(cmd, cfg.Env)
	setupStdin(cmd, cfg.Prompt, recorder)
	bo := setupBufferOutput(cmd, cfg, recorder)

	recordMeta(recorder, g.name, cmdName, args, cfg.WorkDir)

	start := time.Now()
	debug.LogKV("agent.generic", "process starting", "name", g.name, "binary", cmdName)
	err := cmd.Run()
	duration := time.Since(start)

	exitCode, err := extractExitCode(err)
	if err != nil {
		debug.LogKV("agent.generic", "cmd.Run() error (not ExitError)", "name", g.name, "error", err)
		return nil, fmt.Errorf("agent %q: failed to run command: %w", g.name, err)
	}

	debug.LogKV("agent.generic", "process finished",
		"name", g.name,
		"exit_code", exitCode,
		"duration", duration,
		"output_len", bo.StdoutBuf.Len(),
	)

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   bo.StdoutBuf.String(),
		Error:    bo.StderrBuf.String(),
	}, nil
}
