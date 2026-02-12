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

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/stream"
)

// GeminiAgent runs Google's gemini CLI tool.
type GeminiAgent struct{}

// NewGeminiAgent creates a new GeminiAgent.
func NewGeminiAgent() *GeminiAgent {
	return &GeminiAgent{}
}

// Name returns "gemini".
func (g *GeminiAgent) Name() string {
	return "gemini"
}

// Run executes the gemini CLI with the given configuration.
//
// It uses the -p flag to pass the prompt directly as a command-line argument.
// Additional flags such as --model and -y can be supplied via cfg.Args.
//
// Output is streamed in real-time using --output-format stream-json,
// which produces NDJSON events that are parsed, displayed, and recorded.
func (g *GeminiAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "gemini"
	}

	// Build arguments: start with configured defaults, then append streaming
	// flags and the prompt.
	args := make([]string, 0, len(cfg.Args)+4)
	args = append(args, cfg.Args...)
	args = append(args, "--output-format", "stream-json")

	// Pass the prompt via the -p flag.
	if cfg.Prompt != "" {
		args = append(args, "-p", cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

	// Run the command in its own process group so that context cancellation
	// kills the entire tree (important for Node.js-based CLIs that spawn
	// child processes).
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
		return nil, fmt.Errorf("gemini agent: stdout pipe: %w", err)
	}

	// Determine stderr writer.
	stderrW := cfg.Stderr
	if stderrW == nil {
		stderrW = os.Stderr
	}

	// Stderr still goes through the old MultiWriter path.
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(&stderrBuf, recorder.WrapWriter(stderrW, "stderr"))

	recorder.RecordMeta("agent", "gemini")
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("gemini agent: failed to start command: %w", err)
	}

	// Parse the NDJSON stream using the Gemini parser.
	events := stream.ParseGemini(ctx, stdoutPipe)

	var textBuf strings.Builder

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

			// Forward to TUI.
			cfg.EventSink <- ev

			// Accumulate text for Result.Output.
			if ev.Parsed.Type == "assistant" && ev.Parsed.AssistantMessage != nil {
				for _, block := range ev.Parsed.AssistantMessage.Content {
					if block.Type == "text" {
						textBuf.WriteString(block.Text)
					}
				}
			}
			if ev.Parsed.Type == "content_block_delta" &&
				ev.Parsed.Delta != nil &&
				ev.Parsed.Delta.Type == "text_delta" {
				textBuf.WriteString(ev.Parsed.Delta.Text)
			}
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

				display.Handle(ev.Parsed)

				// Accumulate text for Result.Output.
				if ev.Parsed.Type == "assistant" && ev.Parsed.AssistantMessage != nil {
					for _, block := range ev.Parsed.AssistantMessage.Content {
						if block.Type == "text" {
							textBuf.WriteString(block.Text)
						}
					}
				}
				if ev.Parsed.Type == "content_block_delta" &&
					ev.Parsed.Delta != nil &&
					ev.Parsed.Delta.Type == "text_delta" {
					textBuf.WriteString(ev.Parsed.Delta.Text)
				}

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
			return nil, fmt.Errorf("gemini agent: failed to run command: %w", waitErr)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   textBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
