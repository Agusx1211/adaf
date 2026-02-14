package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/recording"
)

// VibeAgent runs the vibe CLI tool.
type VibeAgent struct{}

// NewVibeAgent creates a new VibeAgent.
func NewVibeAgent() *VibeAgent {
	return &VibeAgent{}
}

// Name returns "vibe".
func (v *VibeAgent) Name() string {
	return "vibe"
}

// Run executes the vibe CLI with the given configuration.
//
// The -p flag activates programmatic mode. We prefer passing prompt text via
// stdin to avoid argv size limits, and fall back to argv when stdin mode is
// unsupported by the runtime environment (e.g. no controlling TTY).
// In programmatic mode vibe auto-approves all tool executions and exits
// after completing the response (equivalent to using the "auto-approve"
// agent profile).
//
// We always use --output text because vibe's streaming mode outputs NDJSON
// which requires a stream parser (not yet implemented). Text mode provides
// plain-text output that works with runBufferAgent.
//
// Model selection is done via the VIBE_ACTIVE_MODEL environment variable
// (set in cfg.Env) rather than a --model flag, because vibe uses
// pydantic-settings with env_prefix="VIBE_" to override any config field.
//
// Additional flags (e.g. --max-turns, --max-price) can be supplied via
// cfg.Args.
func (v *VibeAgent) Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error) {
	cmdName := cfg.Command
	if cmdName == "" {
		cmdName = "vibe"
	}

	// Build arguments: start with configured defaults, then append
	// programmatic-mode flags.
	args := make([]string, 0, len(cfg.Args)+6)
	args = append(args, cfg.Args...)

	// Resume a previous session if a session ID is provided.
	if cfg.ResumeSessionID != "" {
		args = append(args, "--resume", cfg.ResumeSessionID)
	}

	var stdinReader io.Reader
	if cfg.Prompt != "" {
		if canUseVibeStdinPrompt() {
			// Passing -p with no value makes vibe read prompt from stdin.
			args = append(args, "-p")
			stdinReader = strings.NewReader(cfg.Prompt)
		} else {
			// Fallback for environments where vibe stdin prompt mode fails
			// (for example when /dev/tty is unavailable).
			args = append(args, "-p", cfg.Prompt)
		}
		// Always use text output mode. Vibe's streaming mode outputs NDJSON
		// which requires a stream parser (not yet implemented). Text mode
		// provides plain-text output that works with runBufferAgent.
		args = append(args, "--output", "text")
		recorder.RecordStdin(cfg.Prompt)
	}

	// Create an isolated VIBE_HOME so session logs are written to a known
	// location with zero collision risk from concurrent or external vibe runs.
	vibeHome, err := os.MkdirTemp("", "adaf-vibe-home-*")
	if err != nil {
		return nil, fmt.Errorf("vibe agent: failed to create temp VIBE_HOME: %w", err)
	}
	defer os.RemoveAll(vibeHome)

	sessionLogDir := filepath.Join(vibeHome, "logs", "session")
	if err := os.MkdirAll(sessionLogDir, 0o755); err != nil {
		return nil, fmt.Errorf("vibe agent: failed to create session log dir: %w", err)
	}

	// Copy user config files into the isolated home so model/provider settings
	// and API keys are available.
	copyVibeConfigFiles(vibeHome)

	// Merge VIBE_HOME into the env overlay.
	if cfg.Env == nil {
		cfg.Env = make(map[string]string)
	}
	cfg.Env["VIBE_HOME"] = vibeHome

	debug.LogKV("agent.vibe", "building command",
		"binary", cmdName,
		"args", strings.Join(args, " "),
		"workdir", cfg.WorkDir,
		"prompt_len", len(cfg.Prompt),
		"vibe_home", vibeHome,
	)

	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = cfg.WorkDir
	if stdinReader != nil {
		cmd.Stdin = stdinReader
	}

	setupProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	setupEnv(cmd, cfg.Env)

	result, err := runBufferAgent(cmd, cfg, recorder, "vibe", cmdName, args)
	if err != nil {
		return nil, err
	}
	result.AgentSessionID = extractVibeSessionID(sessionLogDir)
	return result, nil
}

func canUseVibeStdinPrompt() bool {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}

// copyVibeConfigFiles copies config.toml and .env from the user's default
// ~/.vibe directory into the isolated VIBE_HOME so model/provider settings
// and API keys remain available. Errors are silently ignored (best-effort).
func copyVibeConfigFiles(vibeHome string) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return
	}
	defaultVibeDir := filepath.Join(userHome, ".vibe")
	for _, name := range []string{"config.toml", ".env"} {
		src := filepath.Join(defaultVibeDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			continue // file doesn't exist or isn't readable
		}
		_ = os.WriteFile(filepath.Join(vibeHome, name), data, 0o600)
	}
}

// extractVibeSessionID reads the session ID from the meta.json file inside
// the first session_* subdirectory found in sessionDir. Returns empty string
// on any error (best-effort, non-fatal).
func extractVibeSessionID(sessionDir string) string {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session_") {
			continue
		}
		metaPath := filepath.Join(sessionDir, entry.Name(), "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var meta struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal(data, &meta) == nil && meta.SessionID != "" {
			return meta.SessionID
		}
	}
	return ""
}
