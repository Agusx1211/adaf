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
printf 'args:%s\n' "$*"
if [ "$1" != "exec" ]; then
  echo 'missing exec subcommand' >&2
  exit 9
fi
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
		t.Fatalf("Run() exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Output, "args:exec --skip-git-repo-check --full-auto PING") {
		t.Fatalf("Run() output missing expected arg sequence, got %q", result.Output)
	}
}
