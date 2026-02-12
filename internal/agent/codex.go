package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

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
		// Resume a previous thread using "exec resume --last".
		args = append(args, "exec", "resume", "--last")
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

	// codex exec accepts prompt via positional arg or stdin.
	// We pass prompt through stdin to avoid argv size limits on long prompts.
	var stdinReader io.Reader
	if cfg.Prompt != "" {
		stdinReader = strings.NewReader(cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir
	if stdinReader != nil {
		cmd.Stdin = stdinReader
	}

	// Start the process in its own process group so that context
	// cancellation can kill the entire tree (codex may spawn sandbox
	// children that keep stdout/stderr pipes open).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Kill the entire process group (negative PID).
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
	cmd.Env = withDefaultCodexRustLog(cmd.Env)

	// Set up stdout pipe for streaming JSONL parsing.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex agent: stdout pipe: %w", err)
	}

	stderrW := cfg.Stderr
	if stderrW == nil {
		stderrW = os.Stderr
	}

	var stderrBuf strings.Builder
	stderrWriters := []io.Writer{
		&stderrBuf,
		recorder.WrapWriter(stderrW, "stderr"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, "[stderr] "); w != nil {
		stderrWriters = append(stderrWriters, w)
	}
	cmd.Stderr = io.MultiWriter(stderrWriters...)

	recorder.RecordMeta("agent", "codex")
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex agent: failed to start command: %w", err)
	}

	events := stream.ParseCodex(ctx, stdoutPipe)
	var textBuf strings.Builder
	var agentSessionID string

	accumulateText := func(ev stream.ClaudeEvent) {
		switch ev.Type {
		case "assistant":
			if ev.AssistantMessage != nil {
				for _, block := range ev.AssistantMessage.Content {
					if block.Type == "text" {
						textBuf.WriteString(block.Text)
					}
				}
			}
		case "result":
			if ev.ResultText != "" {
				textBuf.Reset()
				textBuf.WriteString(ev.ResultText)
			}
		}
	}

	if cfg.EventSink != nil {
		for ev := range events {
			if len(ev.Raw) > 0 {
				recorder.RecordStream(string(ev.Raw))
			}
			if ev.Err != nil || ev.Parsed.Type == "" {
				continue
			}
			if ev.Parsed.Type == "system" && ev.Parsed.Subtype == "init" && ev.Parsed.TurnID != "" {
				agentSessionID = ev.Parsed.TurnID
			}
			ev.TurnID = cfg.TurnID
			cfg.EventSink <- ev
			accumulateText(ev.Parsed)
		}
	} else {
		stdoutW := cfg.Stdout
		if stdoutW == nil {
			stdoutW = os.Stdout
		}
		display := stream.NewDisplay(stdoutW)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case ev, ok := <-events:
				if !ok {
					goto done
				}
				if len(ev.Raw) > 0 {
					recorder.RecordStream(string(ev.Raw))
				}
				if ev.Err != nil || ev.Parsed.Type == "" {
					continue
				}
				if ev.Parsed.Type == "system" && ev.Parsed.Subtype == "init" && ev.Parsed.TurnID != "" {
					agentSessionID = ev.Parsed.TurnID
				}
				display.Handle(ev.Parsed)
				accumulateText(ev.Parsed)
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				fmt.Fprintf(stderrW, "\033[2m[status]\033[0m agent running for %s...\n", elapsed)
			}
		}

	done:
		display.Finish()
	}

	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("codex agent: failed to run command: %w", waitErr)
		}
	}

	return &Result{
		ExitCode:       exitCode,
		Duration:       duration,
		Output:         textBuf.String(),
		Error:          stderrBuf.String(),
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
