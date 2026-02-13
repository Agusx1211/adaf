package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stream"
)

const codexDefaultRustLog = "error,codex_core::rollout::list=off"

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
// ADAF runs codex in non-interactive mode via "codex exec" so the underlying
// TUI does not take over the terminal. The exec subcommand defaults to
// never asking for approvals. Additional flags (e.g. --model,
// --dangerously-bypass-approvals-and-sandbox) can be supplied via cfg.Args.
//
// To avoid Codex workspace sandbox restrictions blocking delegated sub-agents,
// ADAF enforces --dangerously-bypass-approvals-and-sandbox unless the caller
// already supplied it (or --yolo).
//
// Output is requested in JSONL mode (--json) and parsed into the common stream
// event format so TUI/CLI rendering is consistent with other stream agents.
func (c *CodexAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "codex"
	}

	// Build arguments: force non-interactive exec mode, then configured flags.
	args := make([]string, 0, len(cfg.Args)+8)
	if cfg.ResumeSessionID != "" {
		// Resume a previous thread by its ID.
		args = append(args, "exec", "resume", cfg.ResumeSessionID)
	} else {
		args = append(args, "exec")
	}

	// Allow running outside a git repository since ADAF manages its own
	// worktrees and launch contexts.
	if !hasFlag(cfg.Args, "--skip-git-repo-check") {
		args = append(args, "--skip-git-repo-check")
	}

	// Full-auto still enables workspace sandboxing. ADAF must run without
	// sandboxing, so remove --full-auto if present and force the danger flag.
	userArgs := withoutFlag(cfg.Args, "--full-auto")
	args = append(args, userArgs...)
	if !hasFlag(userArgs, "--dangerously-bypass-approvals-and-sandbox") && !hasFlag(userArgs, "--yolo") {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	if !hasFlag(userArgs, "--json") && !hasFlag(userArgs, "--experimental-json") {
		args = append(args, "--json")
	}

	debug.LogKV("agent.codex", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
		"resume_session", cfg.ResumeSessionID,
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	setupStdin(cmd, cfg.Prompt, recorder)
	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second

	setupEnv(cmd, cfg.Env)
	cmd.Env = withDefaultCodexRustLog(cmd.Env)

	// Set up stdout pipe for streaming JSONL parsing.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex agent: stdout pipe: %w", err)
	}

	ss := setupStreamStderr(cmd, cfg, recorder)
	recordMeta(recorder, "codex", cmdName, args, cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		debug.LogKV("agent.codex", "process start failed", "error", err)
		return nil, fmt.Errorf("codex agent: failed to start command: %w", err)
	}
	debug.LogKV("agent.codex", "process started", "pid", cmd.Process.Pid)

	events := stream.ParseCodex(ctx, stdoutPipe)
	text, agentSessionID := runStreamLoop(cfg, events, recorder, start, ss.W)

	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode, err := extractExitCode(waitErr)
	if err != nil {
		debug.LogKV("agent.codex", "cmd.Wait() error (not ExitError)", "error", err)
		return nil, fmt.Errorf("codex agent: failed to run command: %w", err)
	}

	debug.LogKV("agent.codex", "process finished",
		"exit_code", exitCode,
		"duration", duration,
		"output_len", len(text),
		"agent_session_id", agentSessionID,
	)

	return &Result{
		ExitCode:       exitCode,
		Duration:       duration,
		Output:         text,
		Error:          ss.Buf.String(),
		AgentSessionID: agentSessionID,
	}, nil
}

// hasFlag returns true if flag appears in args.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// withoutFlag returns a copy of args with exact matches to flag removed.
func withoutFlag(args []string, flag string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == flag {
			continue
		}
		out = append(out, a)
	}
	return out
}

// withDefaultCodexRustLog installs a default log filter unless RUST_LOG is
// already explicitly present in the environment.
func withDefaultCodexRustLog(env []string) []string {
	if hasEnvKey(env, "RUST_LOG") {
		return env
	}
	return append(env, "RUST_LOG="+codexDefaultRustLog)
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}
