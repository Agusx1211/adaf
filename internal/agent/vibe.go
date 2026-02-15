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

	// Use a persistent VIBE_HOME under the project's .adaf directory so that
	// session data survives across runs and --resume can find previous sessions.
	// Vibe natively supports multiple sessions in the same home directory.
	// Fall back to a temp dir if WorkDir is unavailable.
	vibeHome, vibeHomeTmp := vibeHomeDir(cfg.WorkDir)
	if vibeHomeTmp {
		defer os.RemoveAll(vibeHome)
	}

	sessionLogDir := filepath.Join(vibeHome, "logs", "session")
	if err := os.MkdirAll(sessionLogDir, 0o755); err != nil {
		return nil, fmt.Errorf("vibe agent: failed to create session log dir: %w", err)
	}

	// Copy user config files so model/provider settings and API keys
	// are available in the vibe home.
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

	start := time.Now()
	result, err := runBufferAgent(cmd, cfg, recorder, "vibe", cmdName, args)
	if err != nil {
		return nil, err
	}
	result.AgentSessionID = extractVibeSessionID(start, vibeHome)
	return result, nil
}

// vibeHomeDir returns a VIBE_HOME directory path and whether it's a temp dir
// (that the caller should clean up). When a workDir is available, it uses a
// persistent location under .adaf/local/vibe_home/ so session data survives
// across runs. Falls back to a temp dir when workDir is empty.
func vibeHomeDir(workDir string) (string, bool) {
	if workDir != "" {
		dir := filepath.Join(workDir, ".adaf", "local", "vibe_home")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir, false
		}
	}
	dir, err := os.MkdirTemp("", "adaf-vibe-home-*")
	if err != nil {
		// Last resort: use a fixed temp path.
		dir = filepath.Join(os.TempDir(), "adaf-vibe-home-fallback")
		_ = os.MkdirAll(dir, 0o755)
		return dir, false
	}
	return dir, true
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

// extractVibeSessionID finds the vibe session ID created during the current run.
// It searches the active VIBE_HOME path first, then falls back to ~/.vibe.
func extractVibeSessionID(startTime time.Time, vibeHome string) string {
	candidateDirs := []string{}
	seen := make(map[string]struct{})
	addSessionDir := func(dir string) {
		if dir == "" {
			return
		}
		if _, ok := seen[dir]; ok {
			return
		}
		seen[dir] = struct{}{}
		candidateDirs = append(candidateDirs, dir)
	}

	if strings.TrimSpace(vibeHome) != "" {
		addSessionDir(filepath.Join(vibeHome, "logs", "session"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		addSessionDir(filepath.Join(home, ".vibe", "logs", "session"))
	}

	for _, sessionDir := range candidateDirs {
		entries, err := os.ReadDir(sessionDir)
		if err != nil {
			continue
		}

		// Walk entries in reverse order (most recent first, since names are
		// timestamp-sorted) and return the first session created after startTime.
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session_") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// Only consider sessions created during or after our run.
			if info.ModTime().Before(startTime) {
				break // entries are sorted, older ones come before
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
	}
	return ""
}
