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

func TestOpencodePromptUsesStdinNotArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "fake-opencode")
	script := `#!/usr/bin/env sh
expected="PROMPT_SENTINEL_321"
stdin_data="$(cat)"
if [ "$1" != "run" ]; then
	echo "missing run subcommand" >&2
	exit 95
fi
for arg in "$@"; do
	if [ "$arg" = "$expected" ]; then
		echo "prompt passed as argv" >&2
		exit 97
	fi
done
if [ "$stdin_data" != "$expected" ]; then
	echo "stdin mismatch" >&2
	exit 98
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

	result, err := NewOpencodeAgent().Run(context.Background(), Config{
		Command: cmdPath,
		WorkDir: tmp,
		Prompt:  "PROMPT_SENTINEL_321",
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

	events := rec.Events()
	for _, ev := range events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, "command=") && strings.Contains(ev.Data, "PROMPT_SENTINEL_321") {
			t.Fatalf("command metadata leaked prompt into argv: %q", ev.Data)
		}
	}
}
