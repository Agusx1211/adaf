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
printf 'mode:%s\n' "$mode"
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

func TestExtractVibeSessionID(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		want    string
	}{
		{
			name:  "empty directory",
			setup: func(t *testing.T, dir string) {},
			want:  "",
		},
		{
			name: "valid session with meta.json",
			setup: func(t *testing.T, dir string) {
				sessDir := filepath.Join(dir, "session_20250101_abcd1234")
				if err := os.MkdirAll(sessDir, 0o755); err != nil {
					t.Fatal(err)
				}
				meta := `{"session_id": "abcd1234-5678-9abc-def0-123456789abc"}`
				if err := os.WriteFile(filepath.Join(sessDir, "meta.json"), []byte(meta), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "abcd1234-5678-9abc-def0-123456789abc",
		},
		{
			name: "no session_prefix directory",
			setup: func(t *testing.T, dir string) {
				otherDir := filepath.Join(dir, "other_dir")
				if err := os.MkdirAll(otherDir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
		{
			name: "session directory without meta.json",
			setup: func(t *testing.T, dir string) {
				sessDir := filepath.Join(dir, "session_20250101_abcd1234")
				if err := os.MkdirAll(sessDir, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
		{
			name: "invalid JSON in meta.json",
			setup: func(t *testing.T, dir string) {
				sessDir := filepath.Join(dir, "session_20250101_abcd1234")
				if err := os.MkdirAll(sessDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(sessDir, "meta.json"), []byte("not json"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)
			got := extractVibeSessionID(dir)
			if got != tt.want {
				t.Errorf("extractVibeSessionID() = %q, want %q", got, tt.want)
			}
		})
	}
}
