package agent

// helpers.go provides shared infrastructure for agent Run() implementations,
// eliminating boilerplate that was previously duplicated across all agents.
//
// Two patterns are supported:
//   - Stream agents (claude, codex, gemini): pipe stdout through an NDJSON
//     parser, use setupStreamStderr + runStreamLoop.
//   - Buffer agents (vibe, opencode, generic): capture stdout/stderr into
//     buffers, use setupBufferOutput.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/eventq"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stream"
)

const (
	processGroupTerminateGrace = 600 * time.Millisecond
	processGroupTerminatePoll  = 30 * time.Millisecond
)

// --- Command setup helpers ---

// setupEnv configures the command environment by inheriting the current
// process environment and overlaying the provided extra variables.
func setupEnv(cmd *exec.Cmd, env map[string]string) {
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
}

// setupProcessGroup starts the command in its own process group so that
// context cancellation can kill the entire tree. This is required for
// Node.js-based CLIs (claude, codex, gemini, opencode) that spawn child
// processes; without it, orphans will hold pipes open and hang the parent.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		pgid := -cmd.Process.Pid
		// Try graceful shutdown first so stream parsers can flush any final output.
		if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		if waitForProcessGroupExit(pgid, processGroupTerminateGrace, processGroupTerminatePoll) {
			return nil
		}
		if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
}

func waitForProcessGroupExit(pgid int, timeout, pollEvery time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	if pollEvery <= 0 {
		pollEvery = 25 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Kill(pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pollEvery)
	}
}

// setupStdin pipes prompt text to the command's stdin and records it.
// Does nothing if prompt is empty. Agents with non-standard stdin handling
// (e.g. vibe, gemini) should handle this themselves.
func setupStdin(cmd *exec.Cmd, prompt string, recorder *recording.Recorder) {
	if prompt != "" {
		cmd.Stdin = strings.NewReader(prompt)
		recorder.RecordStdin(prompt)
	}
}

// recordMeta writes standard agent metadata entries to the recorder.
func recordMeta(recorder *recording.Recorder, agentName, cmdName string, args []string, workDir string) {
	recorder.RecordMeta("agent", agentName)
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", workDir)
}

// extractExitCode interprets a process error as an exit code.
// Returns (0, nil) for a clean exit, (code, nil) for an ExitError,
// or (0, err) for any other error.
func extractExitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

// writerOrDefault returns w if non-nil, otherwise returns fallback.
func writerOrDefault(w, fallback io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return fallback
}

// --- Argument and environment utilities ---

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

// hasEnvKey returns true if key is present as a KEY=... entry in env.
func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

// --- Buffer agent output capture ---

// bufferOutput holds the in-memory buffers used by buffer-mode agents
// (vibe, opencode, generic) to capture stdout and stderr.
type bufferOutput struct {
	StdoutBuf bytes.Buffer
	StderrBuf bytes.Buffer
}

// setupBufferOutput configures stdout and stderr capture with MultiWriter
// chains that write to in-memory buffers, the recorder, optional cfg writers,
// and optionally the EventSink for TUI mode.
func setupBufferOutput(cmd *exec.Cmd, cfg Config, recorder *recording.Recorder) *bufferOutput {
	bo := &bufferOutput{}
	stdoutW := writerOrDefault(cfg.Stdout, os.Stdout)
	stderrW := writerOrDefault(cfg.Stderr, os.Stderr)

	stdoutWriters := []io.Writer{
		&bo.StdoutBuf,
		recorder.WrapWriter(stdoutW, "stdout"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, ""); w != nil {
		stdoutWriters = append(stdoutWriters, w)
	}

	stderrWriters := []io.Writer{
		&bo.StderrBuf,
		recorder.WrapWriter(stderrW, "stderr"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, "[stderr] "); w != nil {
		stderrWriters = append(stderrWriters, w)
	}

	cmd.Stdout = io.MultiWriter(stdoutWriters...)
	cmd.Stderr = io.MultiWriter(stderrWriters...)
	return bo
}

// --- Stream agent helpers ---

// StreamParser converts a stdout reader into a channel of parsed stream
// events. Each stream agent has its own parser (stream.Parse,
// stream.ParseCodex, stream.ParseGemini, stream.ParseOpencode).
type StreamParser func(ctx context.Context, r io.Reader) <-chan stream.RawEvent

// runStreamAgent executes a pre-configured cmd as a stream agent: pipes
// stdout through the given parser, captures stderr, records metadata, and
// returns a Result. The caller is responsible for building the command
// (args, stdin, env, process group, WaitDelay) before calling this.
func runStreamAgent(
	ctx context.Context,
	cmd *exec.Cmd,
	cfg Config,
	recorder *recording.Recorder,
	agentName string,
	cmdName string,
	args []string,
	parser StreamParser,
) (*Result, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%s agent: stdout pipe: %w", agentName, err)
	}

	ss := setupStreamStderr(cmd, cfg, recorder)
	recordMeta(recorder, agentName, cmdName, args, cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		debug.LogKV("agent."+agentName, "process start failed", "error", err)
		return nil, fmt.Errorf("%s agent: failed to start command: %w", agentName, err)
	}
	debug.LogKV("agent."+agentName, "process started", "pid", cmd.Process.Pid)

	events := parser(ctx, stdoutPipe)
	text, agentSessionID := runStreamLoop(cfg, events, recorder, start, ss.W)

	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode, err := extractExitCode(waitErr)
	if err != nil {
		debug.LogKV("agent."+agentName, "cmd.Wait() error (not ExitError)", "error", err)
		return nil, fmt.Errorf("%s agent: failed to run command: %w", agentName, err)
	}

	debug.LogKV("agent."+agentName, "process finished",
		"exit_code", exitCode,
		"duration", duration,
		"output_len", len(text),
		"stderr_len", ss.Buf.Len(),
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

// streamStderr holds stderr capture state for stream-mode agents.
// Stream agents pipe stdout through an NDJSON parser, so only stderr
// uses the MultiWriter capture pattern.
type streamStderr struct {
	Buf bytes.Buffer
	W   io.Writer // the resolved stderr writer (used for the heartbeat ticker)
}

// setupStreamStderr configures stderr capture for stream-mode agents,
// writing to an in-memory buffer, the recorder, and optionally the
// EventSink.
func setupStreamStderr(cmd *exec.Cmd, cfg Config, recorder *recording.Recorder) *streamStderr {
	ss := &streamStderr{}
	ss.W = writerOrDefault(cfg.Stderr, os.Stderr)

	writers := []io.Writer{
		&ss.Buf,
		recorder.WrapWriter(ss.W, "stderr"),
	}
	if w := newEventSinkWriter(cfg.EventSink, cfg.TurnID, "[stderr] "); w != nil {
		writers = append(writers, w)
	}
	cmd.Stderr = io.MultiWriter(writers...)
	return ss
}

// defaultAccumulateText keeps a concise assistant report from stream events.
// It intentionally drops interim chatter that happened before the most recent
// tool call by resetting on tool-use boundaries. This gives parent handoffs a
// stable "final segment" rather than a full transcript dump.
//
// Event handling:
//   - "assistant": append text blocks; reset first if the event includes tool_use
//   - "user": reset when it carries tool_result blocks
//   - "content_block_start": reset on tool_use block starts
//   - "content_block_delta": append text deltas (best-effort partial streaming)
//   - "result": authoritative final text replaces everything when present
func defaultAccumulateText(ev stream.ClaudeEvent, buf *strings.Builder) {
	appendSegment := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(text)
	}

	switch ev.Type {
	case "assistant":
		if ev.AssistantMessage != nil {
			var (
				sawToolUse bool
				textParts  strings.Builder
			)
			for _, block := range ev.AssistantMessage.Content {
				switch block.Type {
				case "tool_use":
					sawToolUse = true
				case "text":
					textParts.WriteString(block.Text)
				}
			}
			if sawToolUse {
				buf.Reset()
			}
			appendSegment(textParts.String())
		}
	case "user":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				if block.Type == "tool_result" {
					buf.Reset()
					break
				}
			}
		}
	case "content_block_start":
		if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
			buf.Reset()
		}
	case "content_block_delta":
		if ev.Delta != nil && ev.Delta.Type == "text_delta" {
			buf.WriteString(ev.Delta.Text)
		}
	case "result":
		if strings.TrimSpace(ev.ResultText) != "" {
			buf.Reset()
			buf.WriteString(strings.TrimSpace(ev.ResultText))
		}
	}
}

// runStreamLoop consumes parsed stream events, recording them and either
// forwarding to the TUI EventSink or displaying via the legacy terminal
// Display with a 30-second heartbeat ticker.
//
// Returns the accumulated text output and the agent session ID (if captured
// from a system init event).
func runStreamLoop(cfg Config, events <-chan stream.RawEvent, recorder *recording.Recorder, start time.Time, stderrW io.Writer) (text string, sessionID string) {
	var textBuf strings.Builder

	if cfg.EventSink != nil {
		// TUI mode: forward events to the sink channel for the TUI to render.
		dropped := 0
		for ev := range events {
			if len(ev.Raw) > 0 {
				recorder.RecordStream(string(ev.Raw))
			}
			if ev.Err != nil || ev.Parsed.Type == "" {
				continue
			}
			if ev.Parsed.Type == "system" && ev.Parsed.Subtype == "init" && ev.Parsed.TurnID != "" {
				sessionID = ev.Parsed.TurnID
			}
			ev.TurnID = cfg.TurnID
			if !eventq.Offer(cfg.EventSink, ev) {
				dropped++
				if dropped == 1 || dropped%100 == 0 {
					debug.LogKV("agent.stream", "dropping stream event due to backpressure", "turn_id", cfg.TurnID, "dropped", dropped, "event_type", ev.Parsed.Type)
				}
			}
			defaultAccumulateText(ev.Parsed, &textBuf)
		}
	} else {
		// Legacy mode: display formatted events in real-time.
		stdoutW := writerOrDefault(cfg.Stdout, os.Stdout)
		display := stream.NewDisplay(stdoutW)

		// Heartbeat ticker so the user knows the agent is still running.
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case ev, ok := <-events:
				if !ok {
					display.Finish()
					return textBuf.String(), sessionID
				}
				if len(ev.Raw) > 0 {
					recorder.RecordStream(string(ev.Raw))
				}
				if ev.Err != nil || ev.Parsed.Type == "" {
					continue
				}
				if ev.Parsed.Type == "system" && ev.Parsed.Subtype == "init" && ev.Parsed.TurnID != "" {
					sessionID = ev.Parsed.TurnID
				}
				display.Handle(ev.Parsed)
				defaultAccumulateText(ev.Parsed, &textBuf)
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				fmt.Fprintf(stderrW, "\033[2m[status]\033[0m agent running for %s...\n", elapsed)
			}
		}
	}

	return textBuf.String(), sessionID
}
