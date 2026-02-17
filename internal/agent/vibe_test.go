package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

func TestVibePromptUsesStdinNotArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	useStdinPrompt := canUseVibeStdinPrompt()
	cmdPath := filepath.Join(tmp, "fake-vibe")
	script := `#!/usr/bin/env sh
expected="PROMPT_SENTINEL_789"
stdin_data="$(cat)"
has_p=0
has_prompt_arg=0
for arg in "$@"; do
	if [ "$arg" = "-p" ]; then
		has_p=1
	fi
	if [ "$arg" = "$expected" ]; then
		has_prompt_arg=1
	fi
done
if [ "$has_p" -ne 1 ]; then
	echo "missing -p" >&2
	exit 96
fi
if [ "$has_prompt_arg" -eq 1 ] && [ "$stdin_data" = "$expected" ]; then
	echo "prompt was sent via both argv and stdin" >&2
	exit 99
fi
if [ "$has_prompt_arg" -eq 0 ] && [ "$stdin_data" != "$expected" ]; then
	echo "stdin mode expected but stdin mismatch" >&2
	exit 98
fi
if [ "$has_prompt_arg" -eq 1 ] && [ "$stdin_data" != "" ]; then
	echo "argv mode expected empty stdin" >&2
	exit 95
fi
mode="stdin"
if [ "$has_prompt_arg" -eq 1 ]; then
	mode="argv"
fi
printf '{"role":"assistant","content":"mode:%s ok"}\n' "$mode"
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewVibeAgent().Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "PROMPT_SENTINEL_789",
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
	}
	if !strings.Contains(result.Output, "ok") {
		t.Errorf("Run() output = %q, want to contain %q", result.Output, "ok")
	}
	if useStdinPrompt {
		if !strings.Contains(result.Output, "mode:stdin") {
			t.Fatalf("expected stdin prompt mode, got output %q", result.Output)
		}
	} else {
		if !strings.Contains(result.Output, "mode:argv") {
			t.Fatalf("expected argv fallback mode, got output %q", result.Output)
		}
	}

	events := rec.Events()
	for _, ev := range events {
		if useStdinPrompt && ev.Type == "meta" && strings.HasPrefix(ev.Data, "command=") && strings.Contains(ev.Data, "PROMPT_SENTINEL_789") {
			t.Fatalf("command metadata leaked prompt into argv: %q", ev.Data)
		}
	}
}

func TestVibeRunBasic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-vibe")
	script := `#!/usr/bin/env sh
echo '{"role":"assistant","content":"Hello from vibe"}'
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	t.Run("happy path", func(t *testing.T) {
		result, err := NewVibeAgent().Run(context.Background(), Config{
			Command: cmdPath,
			WorkDir: tmp,
			Prompt:  "hello",
		}, rec)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("Run() exit code = %d, want 0", result.ExitCode)
		}
		if !strings.Contains(result.Output, "Hello from vibe") {
			t.Errorf("Run() output = %q, want to contain %q", result.Output, "Hello from vibe")
		}
	})

	t.Run("non-zero exit", func(t *testing.T) {
		failPath := filepath.Join(tmp, "fake-vibe-fail")
		failScript := `#!/usr/bin/env sh
echo "vibe failed" >&2
exit 42
`
		if err := os.WriteFile(failPath, []byte(failScript), 0755); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		result, err := NewVibeAgent().Run(context.Background(), Config{
			Command: failPath,
			WorkDir: tmp,
			Prompt:  "fail",
		}, rec)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 42 {
			t.Errorf("Run() exit code = %d, want 42", result.ExitCode)
		}
		if !strings.Contains(result.Error, "vibe failed") {
			t.Errorf("Run() stderr = %q, want to contain %q", result.Error, "vibe failed")
		}
	})
}

func TestVibeHomeDir_PersistentWhenWorkDirSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	s, err := store.New(workDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "vibe-home-test", RepoPath: workDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	dir, isTmp := vibeHomeDir(workDir)
	if isTmp {
		t.Fatal("vibeHomeDir with workDir should return persistent dir, got temp")
	}

	expected := filepath.Join(s.Root(), "local", "vibe_home")
	if dir != expected {
		t.Fatalf("vibeHomeDir = %q, want %q", dir, expected)
	}

	// Directory should exist.
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("persistent vibeHome dir should exist: %v", err)
	}
}

func TestVibeHomeDir_TempWhenNoWorkDir(t *testing.T) {
	dir, isTmp := vibeHomeDir("")
	if !isTmp {
		t.Fatal("vibeHomeDir with empty workDir should return temp dir")
	}
	defer os.RemoveAll(dir)

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp vibeHome dir should exist: %v", err)
	}
}

func TestExtractVibeSessionID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionDir := filepath.Join(home, ".vibe", "logs", "session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("empty directory", func(t *testing.T) {
		got := extractVibeSessionID(time.Now().Add(-time.Hour), filepath.Join(home, ".vibe"))
		if got != "" {
			t.Errorf("extractVibeSessionID() = %q, want empty", got)
		}
	})

	t.Run("custom vibe home is preferred", func(t *testing.T) {
		before := time.Now().Add(-time.Second)
		customHome := filepath.Join(home, "custom_vibe_home")
		customSessionDir := filepath.Join(customHome, "logs", "session")
		if err := os.MkdirAll(customSessionDir, 0o755); err != nil {
			t.Fatal(err)
		}

		sessDir := filepath.Join(customSessionDir, "session_20260215_120000_abcd")
		if err := os.MkdirAll(sessDir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta := `{"session_id": "deadbeef-1111-2222-3333-444444444444"}`
		if err := os.WriteFile(filepath.Join(sessDir, "meta.json"), []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}

		got := extractVibeSessionID(before, customHome)
		if got != "deadbeef-1111-2222-3333-444444444444" {
			t.Errorf("extractVibeSessionID() = %q, want custom home session", got)
		}
	})

	t.Run("valid session after start time", func(t *testing.T) {
		before := time.Now().Add(-time.Second)
		sessDir := filepath.Join(sessionDir, "session_20260215_100000_abcd1234")
		if err := os.MkdirAll(sessDir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta := `{"session_id": "abcd1234-5678-9abc-def0-123456789abc"}`
		if err := os.WriteFile(filepath.Join(sessDir, "meta.json"), []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}
		got := extractVibeSessionID(before, filepath.Join(home, ".vibe"))
		if got != "abcd1234-5678-9abc-def0-123456789abc" {
			t.Errorf("extractVibeSessionID() = %q, want %q", got, "abcd1234-5678-9abc-def0-123456789abc")
		}
	})

	t.Run("session before start time is skipped", func(t *testing.T) {
		got := extractVibeSessionID(time.Now().Add(time.Hour), filepath.Join(home, ".vibe"))
		if got != "" {
			t.Errorf("extractVibeSessionID(future) = %q, want empty", got)
		}
	})

	t.Run("invalid JSON in meta.json", func(t *testing.T) {
		before := time.Now().Add(-time.Second)
		sessDir := filepath.Join(sessionDir, "session_20260215_110000_bad12345")
		if err := os.MkdirAll(sessDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sessDir, "meta.json"), []byte("not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Should still find the valid session from the previous subtest.
		got := extractVibeSessionID(before, filepath.Join(home, ".vibe"))
		if got != "abcd1234-5678-9abc-def0-123456789abc" {
			t.Errorf("extractVibeSessionID() = %q, want valid session from earlier", got)
		}
	})
}

func TestVibeRunCancellationPreservesSessionID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}
	t.Setenv("HOME", t.TempDir())

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-vibe-cancel")
	script := `#!/usr/bin/env sh
trap 'exit 0' TERM
session_dir="$VIBE_HOME/logs/session/session_20260216_000000_test"
mkdir -p "$session_dir"
cat >"$session_dir/meta.json" <<'EOF'
{"session_id":"vibe-cancel-session"}
EOF
cat >/dev/null
printf '{"role":"assistant","content":"running"}\n'
while true; do sleep 1; done
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "vibe-cancel-test", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	rec := recording.New(1, s)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := NewVibeAgent().Run(ctx, Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "PING",
	}, rec)

	if err == nil {
		t.Fatal("Run() error = nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context canceled/deadline exceeded", err)
	}
	if result == nil {
		t.Fatal("Run() result is nil, want partial result with session ID")
	}
	if got, want := result.AgentSessionID, "vibe-cancel-session"; got != want {
		t.Fatalf("Run() AgentSessionID = %q, want %q", got, want)
	}
}
