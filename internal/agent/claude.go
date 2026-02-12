package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stream"
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
// It uses --print (-p) to enable non-interactive mode and passes the prompt
// via stdin. Additional flags such as --model and
// --dangerously-skip-permissions can be supplied via cfg.Args.
//
// Output is streamed in real-time using --output-format stream-json --verbose,
// which produces NDJSON events that are parsed, displayed, and recorded.
// The --verbose flag is required when using --output-format stream-json.
func (c *ClaudeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "claude"
	}

	// Build arguments: start with configured defaults, then append streaming
	// flags.
	args := make([]string, 0, len(cfg.Args)+5)
	args = append(args, cfg.Args...)

	// --print (-p) enables non-interactive mode (print response and exit).
	// --output-format stream-json produces NDJSON events on stdout.
	// --verbose is required by the CLI when using stream-json output format.
	args = append(args, "--print", "--output-format", "stream-json", "--verbose")

	// Resume a previous session if a session ID is provided.
	if cfg.ResumeSessionID != "" {
		args = append(args, "--resume", cfg.ResumeSessionID)
	}

	// In --print mode, Claude accepts prompt via stdin or positional arg.
	// We pass prompt through stdin to avoid argv size limits on long prompts.
	var stdinReader io.Reader
	if cfg.Prompt != "" {
		stdinReader = strings.NewReader(cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	debug.LogKV("agent.claude", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
		"resume_session", cfg.ResumeSessionID,
		"has_event_sink", cfg.EventSink != nil,
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir
	if stdinReader != nil {
		cmd.Stdin = stdinReader
	}

	// Run the command in its own process group so that context cancellation
	// kills the entire tree. Claude Code is Node.js-based and spawns child
	// processes for tool use (Bash, etc.); without Setpgid + process group
	// kill, orphan processes will hold pipes open and hang the parent.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Send SIGKILL to the entire process group (negative PID).
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}

	// Environment: inherit + overlay.
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Set up stdout pipe for streaming NDJSON parsing.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("claude agent: stdout pipe: %w", err)
	}

	// Determine stderr writer.
	stderrW := cfg.Stderr
	if stderrW == nil {
		stderrW = os.Stderr
	}

	// Stderr still goes through the old MultiWriter path.
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(&stderrBuf, recorder.WrapWriter(stderrW, "stderr"))

	recorder.RecordMeta("agent", "claude")
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		debug.LogKV("agent.claude", "process start failed", "error", err)
		return nil, fmt.Errorf("claude agent: failed to start command: %w", err)
	}
	debug.LogKV("agent.claude", "process started", "pid", cmd.Process.Pid)

	// Parse the NDJSON stream.
	events := stream.Parse(ctx, stdoutPipe)

	var textBuf strings.Builder
	var agentSessionID string

	// accumulateText extracts text content from a stream event.
	// In stream-json mode, the CLI emits complete "assistant" events (not
	// incremental content_block_delta events). The "result" event contains
	// the final authoritative text in its "result" field.
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
		case "content_block_delta":
			// Only present when --include-partial-messages is used.
			if ev.Delta != nil && ev.Delta.Type == "text_delta" {
				textBuf.WriteString(ev.Delta.Text)
			}
		case "result":
			// The result event contains the final text. If present, use it
			// as the authoritative output (replacing accumulated text).
			if ev.ResultText != "" {
				textBuf.Reset()
				textBuf.WriteString(ev.ResultText)
			}
		}
	}

	if cfg.EventSink != nil {
		// TUI mode: forward events to the sink channel for the TUI to render.
		for ev := range events {
			// Record raw NDJSON line.
			if len(ev.Raw) > 0 {
				recorder.RecordStream(string(ev.Raw))
			}

			if ev.Err != nil {
				continue
			}

			// Capture session ID from init event.
			if ev.Parsed.Type == "system" && ev.Parsed.Subtype == "init" && ev.Parsed.TurnID != "" {
				agentSessionID = ev.Parsed.TurnID
			}

			// Forward to TUI.
			cfg.EventSink <- ev

			accumulateText(ev.Parsed)
		}
	} else {
		// Legacy mode: display formatted events in real-time.
		stdoutW := cfg.Stdout
		if stdoutW == nil {
			stdoutW = os.Stdout
		}
		display := stream.NewDisplay(stdoutW)

		// Status ticker: print a heartbeat every 30 seconds so the user
		// knows the agent is still running.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case ev, ok := <-events:
				if !ok {
					goto done
				}

				// Record raw NDJSON line.
				if len(ev.Raw) > 0 {
					recorder.RecordStream(string(ev.Raw))
				}

				if ev.Err != nil {
					continue
				}

				// Capture session ID from init event.
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
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			debug.LogKV("agent.claude", "cmd.Wait() error (not ExitError)", "error", waitErr)
			return nil, fmt.Errorf("claude agent: failed to run command: %w", waitErr)
		}
	}

	debug.LogKV("agent.claude", "process finished",
		"exit_code", exitCode,
		"duration", duration,
		"output_len", textBuf.Len(),
		"stderr_len", stderrBuf.Len(),
		"agent_session_id", agentSessionID,
	)

	return &Result{
		ExitCode:       exitCode,
		Duration:       duration,
		Output:         textBuf.String(),
		Error:          stderrBuf.String(),
		AgentSessionID: agentSessionID,
	}, nil
}
