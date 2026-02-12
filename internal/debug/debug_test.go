package debug

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestShouldEnableFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		enabled string
		path    string
		want    bool
	}{
		{name: "disabled by default", enabled: "", path: "", want: false},
		{name: "enabled explicit", enabled: "1", path: "", want: true},
		{name: "enabled via path", enabled: "", path: "/tmp/adaf.log", want: true},
		{name: "explicit off wins", enabled: "0", path: "/tmp/adaf.log", want: false},
		{name: "unknown toggle without path", enabled: "maybe", path: "", want: false},
		{name: "unknown toggle with path", enabled: "maybe", path: "/tmp/adaf.log", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvEnabled, tt.enabled)
			t.Setenv(EnvLogPath, tt.path)
			if got := ShouldEnableFromEnv(); got != tt.want {
				t.Fatalf("ShouldEnableFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInitInheritedPathAndProcessMetadata(t *testing.T) {
	defer Close()

	logPath := filepath.Join(t.TempDir(), "aggregate.log")
	if err := os.WriteFile(logPath, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv(EnvLogPath, logPath)
	t.Setenv(EnvProcess, "session-daemon:51")

	gotPath, err := Init()
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if gotPath != logPath {
		t.Fatalf("Init() path = %q, want %q", gotPath, logPath)
	}

	LogKV("test", "hello", "k", "v")
	Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "existing\n") {
		t.Fatalf("expected existing file content to remain at beginning, got %q", s)
	}
	if !strings.Contains(s, "=== ADAF DEBUG PROCESS ATTACHED ===") {
		t.Fatalf("missing attach header: %q", s)
	}
	if !strings.Contains(s, "Process: session-daemon:51") {
		t.Fatalf("missing process label in header: %q", s)
	}
	if !strings.Contains(s, "[P") || !strings.Contains(s, "session-daemon:51") {
		t.Fatalf("missing per-line process metadata: %q", s)
	}
	if !strings.Contains(s, "[test") || !strings.Contains(s, "hello k=v") {
		t.Fatalf("missing emitted debug line: %q", s)
	}
	if !strings.Contains(s, "=== DEBUG LOG CLOSED ===") {
		t.Fatalf("missing close marker: %q", s)
	}
}

func TestPropagatedEnv(t *testing.T) {
	t.Run("no debug enabled", func(t *testing.T) {
		defer Close()
		in := []string{"FOO=bar"}
		out := PropagatedEnv(in, "daemon:1")
		if !reflect.DeepEqual(out, in) {
			t.Fatalf("PropagatedEnv() changed env unexpectedly: got=%v want=%v", out, in)
		}
	})

	t.Run("overlay debug vars", func(t *testing.T) {
		defer Close()
		logPath := filepath.Join(t.TempDir(), "shared.log")
		t.Setenv(EnvLogPath, logPath)
		t.Setenv(EnvProcess, "cli:run")
		if _, err := Init(); err != nil {
			t.Fatalf("Init: %v", err)
		}

		out := PropagatedEnv([]string{
			"FOO=bar",
			EnvEnabled + "=0",
			EnvProcess + "=old",
		}, "session-daemon:7")

		m := envMap(out)
		if m["FOO"] != "bar" {
			t.Fatalf("FOO = %q, want bar", m["FOO"])
		}
		if m[EnvEnabled] != "1" {
			t.Fatalf("%s = %q, want 1", EnvEnabled, m[EnvEnabled])
		}
		if m[EnvLogPath] != logPath {
			t.Fatalf("%s = %q, want %q", EnvLogPath, m[EnvLogPath], logPath)
		}
		if m[EnvProcess] != "session-daemon:7" {
			t.Fatalf("%s = %q, want session-daemon:7", EnvProcess, m[EnvProcess])
		}
	})
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		m[parts[0]] = parts[1]
	}
	return m
}
