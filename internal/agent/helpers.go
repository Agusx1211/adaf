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
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
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
