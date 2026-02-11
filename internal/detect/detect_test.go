package detect

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "semver", input: "codex 0.15.2", want: "0.15.2"},
		{name: "prefixed", input: "Claude CLI v1.3.0-beta.1", want: "1.3.0-beta.1"},
		{name: "fallback first line", input: "version unknown\nextra", want: "version unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersion(tt.input)
			if got != tt.want {
				t.Fatalf("parseVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScanDetectsKnownAndGenericAgents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based detection test is unix-only")
	}

	tmp := t.TempDir()

	mustWriteVersionScript(t, filepath.Join(tmp, "claude"), "claude", "1.0.0")
	mustWriteVersionScript(t, filepath.Join(tmp, "codex"), "codex", "2.0.0")
	mustWriteVersionScript(t, filepath.Join(tmp, "vibe"), "vibe", "3.0.0")
	mustWriteVersionScript(t, filepath.Join(tmp, "opencode"), "opencode", "4.0.0")
	mustWriteVersionScript(t, filepath.Join(tmp, "aider"), "aider", "5.0.0")

	oldPath := os.Getenv("PATH")
	oldExtra := os.Getenv("ADAF_EXTRA_AGENT_BINS")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("ADAF_EXTRA_AGENT_BINS", oldExtra)
	})

	if err := os.Setenv("PATH", tmp); err != nil {
		t.Fatalf("setting PATH: %v", err)
	}
	if err := os.Setenv("ADAF_EXTRA_AGENT_BINS", ""); err != nil {
		t.Fatalf("setting ADAF_EXTRA_AGENT_BINS: %v", err)
	}

	agents, err := Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	index := make(map[string]DetectedAgent, len(agents))
	for _, a := range agents {
		index[a.Name] = a
	}

	for _, name := range []string{"claude", "codex", "vibe", "opencode", "aider"} {
		rec, ok := index[name]
		if !ok {
			t.Fatalf("expected %s to be detected", name)
		}
		if rec.Path == "" {
			t.Fatalf("expected %s to have a path", name)
		}
		if rec.Version == "unknown" || rec.Version == "" {
			t.Fatalf("expected %s to have parsed version, got %q", name, rec.Version)
		}
	}
}

func mustWriteVersionScript(t *testing.T, path, name, version string) {
	t.Helper()

	content := "#!/usr/bin/env bash\n" +
		"if [ \"$1\" = \"--version\" ] || [ \"$1\" = \"-v\" ] || [ \"$1\" = \"version\" ]; then\n" +
		"  echo \"" + name + " " + version + "\"\n" +
		"  exit 0\n" +
		"fi\n" +
		"echo \"ok\"\n"

	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("writing script %s: %v", path, err)
	}
}
