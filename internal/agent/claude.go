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
// It uses the -p flag to pass the prompt directly as a command-line argument.
// Additional flags such as --model and --dangerously-skip-permissions can be
// supplied via cfg.Args.
//
// Output is streamed in real-time using --output-format stream-json --verbose,
// which produces NDJSON events that are parsed, displayed, and recorded.
func (c *ClaudeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "claude"
	}

	// Build arguments: start with configured defaults, then append streaming
	// flags and the prompt.
	args := make([]string, 0, len(cfg.Args)+6)
	args = append(args, cfg.Args...)
	args = append(args, "--output-format", "stream-json", "--verbose")

	// Pass the prompt via the -p flag so claude treats it as a non-interactive
	// prompt. If the prompt is empty we still run (claude may read from stdin
	// or use its own interactive mode).
	if cfg.Prompt != "" {
		args = append(args, "-p", cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir

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

	// Stderr still goes through the old MultiWriter path.
	var stderrBuf bytes.Buffer
	cmd.Stderr = io.MultiWriter(&stderrBuf, recorder.WrapWriter(os.Stderr, "stderr"))

	recorder.RecordMeta("agent", "claude")
	recorder.RecordMeta("command", cmdName+" "+strings.Join(args, " "))
	recorder.RecordMeta("workdir", cfg.WorkDir)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude agent: failed to start command: %w", err)
	}

	// Parse the NDJSON stream and display formatted events in real-time.
	display := stream.NewDisplay(os.Stdout)
	events := stream.Parse(ctx, stdoutPipe)

	var textBuf strings.Builder

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
				// Parse error on a line - record but continue.
				continue
			}

			display.Handle(ev.Parsed)

			// Accumulate text from assistant messages for the final Result.Output.
			if ev.Parsed.Type == "assistant" && ev.Parsed.AssistantMessage != nil {
				for _, block := range ev.Parsed.AssistantMessage.Content {
					if block.Type == "text" {
						textBuf.WriteString(block.Text)
					}
				}
			}
			// Also accumulate from content_block_delta (streaming mode).
			if ev.Parsed.Type == "content_block_delta" &&
				ev.Parsed.Delta != nil &&
				ev.Parsed.Delta.Type == "text_delta" {
				textBuf.WriteString(ev.Parsed.Delta.Text)
			}

		case <-ticker.C:
			elapsed := time.Since(start).Round(time.Second)
			fmt.Fprintf(os.Stderr, "\033[2m[status]\033[0m agent running for %s...\n", elapsed)
		}
	}

done:
	display.Finish()

	waitErr := cmd.Wait()
	duration := time.Since(start)

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("claude agent: failed to run command: %w", waitErr)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Duration: duration,
		Output:   textBuf.String(),
		Error:    stderrBuf.String(),
	}, nil
}
