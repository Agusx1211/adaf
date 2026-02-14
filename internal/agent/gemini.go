package agent

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
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
// It passes prompt text via stdin (with an empty -p flag value to force
// headless mode) to avoid argv size limits on long prompts.
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
	// flags. Prompt (if any) is passed via stdin.
	args := make([]string, 0, len(cfg.Args)+6)
	args = append(args, cfg.Args...)
	args = append(args, "--output-format", "stream-json")

	// Resume a previous session if a session ID is provided.
	if cfg.ResumeSessionID != "" {
		args = append(args, "--resume", cfg.ResumeSessionID)
	}

	// Keep -p with an empty value to force non-interactive mode while
	// avoiding long prompt text in argv.
	var stdinReader io.Reader
	if cfg.Prompt != "" {
		args = append(args, "-p", "")
		stdinReader = strings.NewReader(cfg.Prompt)
		recorder.RecordStdin(cfg.Prompt)
	}

	debug.LogKV("agent.gemini", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
		"resume_session", cfg.ResumeSessionID,
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir
	if stdinReader != nil {
		cmd.Stdin = stdinReader
	}

	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	setupEnv(cmd, cfg.Env)

	return runStreamAgent(ctx, cmd, cfg, recorder, "gemini", cmdName, args, stream.ParseGemini)
}
