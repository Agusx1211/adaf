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

func TestGenericRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-generic")
	script := `#!/usr/bin/env sh
echo "Hello from generic"
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewGenericAgent("test-agent").Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Output, "Hello from generic") {
		t.Errorf("Run() output = %q, want to contain %q", result.Output, "Hello from generic")
	}
}

func TestGenericRunNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-generic-fail")
	script := `#!/usr/bin/env sh
echo "failed" >&2
exit 42
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewGenericAgent("test-agent").Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("Run() exit code = %d, want 42", result.ExitCode)
	}
	if !strings.Contains(result.Error, "failed") {
		t.Errorf("Run() stderr = %q, want to contain %q", result.Error, "failed")
	}
}

func TestGenericPromptUsesStdinNotArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-generic-stdin")
	script := `#!/usr/bin/env sh
expected="PROMPT_SENTINEL_123"
stdin_data="$(cat)"
if [ "$stdin_data" != "$expected" ]; then
	echo "stdin mismatch" >&2
	exit 98
fi
for arg in "$@"; do
	if [ "$arg" = "$expected" ]; then
		echo "prompt passed as argv" >&2
		exit 97
	fi
done
echo "ok"
`
	if err := os.WriteFile(cmdPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	result, err := NewGenericAgent("test-agent").Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "PROMPT_SENTINEL_123",
	}, rec)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0; stderr = %q", result.ExitCode, result.Error)
	}

	events := rec.Events()
	for _, ev := range events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, "command=") && strings.Contains(ev.Data, "PROMPT_SENTINEL_123") {
			t.Fatalf("command metadata leaked prompt into argv: %q", ev.Data)
		}
	}
}

func TestGenericNoCommandError(t *testing.T) {
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	rec := recording.New(1, s)

	_, err = NewGenericAgent("test-agent").Run(context.Background(), Config{
		Command: "",
	}, rec)
	if err == nil {
		t.Fatal("Run() expected error with empty command, got nil")
	}
	if !strings.Contains(err.Error(), "no command configured") {
		t.Errorf("Run() error = %v, want to contain %q", err, "no command configured")
	}
}
