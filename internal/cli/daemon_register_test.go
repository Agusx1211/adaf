package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutoRegisterProjectAddsProjectOnce(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	state := webRuntimeState{
		PID:    os.Getpid(),
		URL:    "http://127.0.0.1:8080",
		Port:   8080,
		Host:   "127.0.0.1",
		Scheme: "http",
	}
	if err := writeWebRuntimeFiles(webPIDFilePath(), webStateFilePath(), state); err != nil {
		t.Fatalf("writing runtime files: %v", err)
	}

	projectDir := filepath.Join(t.TempDir(), "example-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	autoRegisterProject(projectDir)
	autoRegisterProject(projectDir)

	registry, err := loadWebProjectRegistry(webProjectsRegistryPath())
	if err != nil {
		t.Fatalf("loading registry: %v", err)
	}
	if len(registry.Projects) != 1 {
		t.Fatalf("registry project count = %d, want 1", len(registry.Projects))
	}
	if filepath.Clean(registry.Projects[0].Path) != filepath.Clean(projectDir) {
		t.Fatalf("registry path = %q, want %q", registry.Projects[0].Path, projectDir)
	}
}
