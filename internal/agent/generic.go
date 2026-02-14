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

	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	setupEnv(cmd, cfg.Env)
	setupStdin(cmd, cfg.Prompt, recorder)

	return runBufferAgent(cmd, cfg, recorder, g.name, cmdName, args)
}
