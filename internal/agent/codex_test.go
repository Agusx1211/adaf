package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
