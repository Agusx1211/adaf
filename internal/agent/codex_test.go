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

func TestCodexRunUsesNonInteractiveExecMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-codex")
	script := `#!/usr/bin/env sh
prompt="$(cat)"
printf 'args:%s\n' "$*"
if [ "$1" != "exec" ]; then
  echo 'missing exec subcommand' >&2
  exit 9
fi
if [ "$prompt" != "PING" ]; then
  echo 'stdin mismatch' >&2
  exit 8
fi
for arg in "$@"; do
  if [ "$arg" = "PING" ]; then
    echo 'prompt passed as argv' >&2
    exit 7
  fi
done
printf 'ok\n'
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewCodexAgent().Run(context.Background(), Config{
		Command: cmdPath,
		Args:    []string{"--full-auto"},
		WorkDir: tmp,
		Prompt:  "PING",
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("Run() result is nil")
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
	}
	if strings.Contains(result.Output, "PING") {
		t.Fatalf("Run() output should not contain prompt in args, got %q", result.Output)
	}

	events := rec.Events()
	foundCommandMeta := false
	for _, ev := range events {
		if ev.Type != "meta" || !strings.HasPrefix(ev.Data, "command=") {
			continue
		}
		foundCommandMeta = true
		if strings.Contains(ev.Data, "PING") {
			t.Fatalf("command metadata leaked prompt into argv: %q", ev.Data)
		}
		if !strings.Contains(ev.Data, "exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox --json") {
			t.Fatalf("command metadata missing expected arg sequence: %q", ev.Data)
		}
		if strings.Contains(ev.Data, "--full-auto") {
			t.Fatalf("command metadata should not include --full-auto: %q", ev.Data)
		}
	}
	if !foundCommandMeta {
		t.Fatal("expected command metadata event")
	}
}

func TestWithDefaultCodexRustLog(t *testing.T) {
	t.Run("adds default when missing", func(t *testing.T) {
		env := []string{"FOO=bar"}
		got := withDefaultCodexRustLog(env)
		if !hasEnvKey(got, "RUST_LOG") {
			t.Fatalf("expected RUST_LOG to be added, got %v", got)
		}
		want := "RUST_LOG=" + codexDefaultRustLog
		found := false
		for _, kv := range got {
			if kv == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected default RUST_LOG %q, got %v", want, got)
		}
	})

	t.Run("respects existing value", func(t *testing.T) {
		env := []string{"RUST_LOG=debug", "FOO=bar"}
		got := withDefaultCodexRustLog(env)
		count := 0
		for _, kv := range got {
			if strings.HasPrefix(kv, "RUST_LOG=") {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected one RUST_LOG entry, got %d (%v)", count, got)
		}
		if got[0] != "RUST_LOG=debug" {
			t.Fatalf("expected existing RUST_LOG to be preserved, got %v", got)
		}
	})
}

func TestCodexRunRustLogDefaultAndOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-codex-env")
	script := `#!/usr/bin/env sh
echo "RUST_LOG=$RUST_LOG" >&2
printf '{"type":"thread.started","thread_id":"test-thread"}\n'
printf '{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1}}\n'
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Run("default applied when missing", func(t *testing.T) {
		s, err := store.New(t.TempDir())
		if err != nil {
			t.Fatalf("store.New() error = %v", err)
		}
		rec := recording.New(1, s)

		result, err := NewCodexAgent().Run(context.Background(), Config{
			Command: cmdPath,
			WorkDir: tmp,
			Prompt:  "PING",
		}, rec)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
		}
		want := "RUST_LOG=" + codexDefaultRustLog
		if !strings.Contains(result.Error, want) {
			t.Fatalf("stderr missing default RUST_LOG, want %q in %q", want, result.Error)
		}
	})

	t.Run("user override preserved", func(t *testing.T) {
		s, err := store.New(t.TempDir())
		if err != nil {
			t.Fatalf("store.New() error = %v", err)
		}
		rec := recording.New(1, s)

		result, err := NewCodexAgent().Run(context.Background(), Config{
			Command: cmdPath,
			WorkDir: tmp,
			Prompt:  "PING",
			Env: map[string]string{
				"RUST_LOG": "debug",
			},
		}, rec)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
		}
		if !strings.Contains(result.Error, "RUST_LOG=debug") {
			t.Fatalf("stderr missing overridden RUST_LOG in %q", result.Error)
		}
		if strings.Contains(result.Error, "RUST_LOG="+codexDefaultRustLog) {
			t.Fatalf("stderr should not contain default RUST_LOG when overridden: %q", result.Error)
		}
	})
}

func TestCodexRunCancellationPreservesSessionID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-codex-cancel")
	script := `#!/usr/bin/env sh
trap 'exit 0' TERM
cat >/dev/null
printf '{"type":"thread.started","thread_id":"cancel-thread"}\n'
while true; do sleep 1; done
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := NewCodexAgent().Run(ctx, Config{
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
	if got, want := result.AgentSessionID, "cancel-thread"; got != want {
		t.Fatalf("Run() AgentSessionID = %q, want %q", got, want)
	}
}
