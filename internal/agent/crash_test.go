package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

// TestGenericAgent_ProcessCrash tests that the generic agent handles process crashes gracefully
func TestGenericAgent_ProcessCrash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()

	// Create a script that crashes after writing some output
	script := filepath.Join(tmp, "crash.sh")
	scriptContent := `#!/usr/bin/env sh
# Write some output before crashing
echo "starting process"
echo "process running"
# Simulate a crash by killing ourselves with SIGKILL
kill -9 $$
`
	if err := os.WriteFile(script, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Setup store and recorder
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	rec := recording.New(1, s)

	// Run the generic agent with the crashing script
	result, err := NewGenericAgent("test-crash-agent").Run(context.Background(), Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  "test prompt",
	}, rec)

	// Verify that we get an error or non-zero exit code
	if err == nil && result.ExitCode == 0 {
		t.Error("Expected error or non-zero exit code for crashed process")
	}

	// Verify that the process didn't hang and returned cleanly
	// (the test should complete within a reasonable time)

	// Verify that we captured some output before the crash
	if !strings.Contains(result.Output, "starting process") {
		t.Errorf("Expected output to contain 'starting process', got: %q", result.Output)
	}
}

// TestGenericAgent_ContextCancellation tests that context cancellation works correctly
func TestGenericAgent_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()

	// Create a script that runs forever
	script := filepath.Join(tmp, "forever.sh")
	scriptContent := `#!/usr/bin/env sh
# Run forever until killed
while true; do
    sleep 1
    echo "still running..."
done
`
	if err := os.WriteFile(script, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Setup store and recorder
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	rec := recording.New(1, s)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run the generic agent with the forever script
	result, err := NewGenericAgent("test-cancel-agent").Run(ctx, Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  "test prompt",
	}, rec)

	// Verify that the context cancellation worked
	// The command should be terminated by the context timeout
	if err == nil && result.ExitCode == 0 {
		t.Error("Expected error or non-zero exit code for cancelled process")
	}

	// The test should complete quickly (within the timeout + some overhead)
	// If it hangs, the test will timeout and fail
}

// TestGenericAgent_StderrCapture tests that stderr from a crashing process is captured
func TestGenericAgent_StderrCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()

	// Create a script that writes to stderr and exits with error
	script := filepath.Join(tmp, "stderr.sh")
	scriptContent := `#!/usr/bin/env sh
# Write error message to stderr
echo "error message" >&2
# Exit with non-zero code
exit 1
`
	if err := os.WriteFile(script, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Setup store and recorder
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	rec := recording.New(1, s)

	// Run the generic agent with the stderr script
	result, err := NewGenericAgent("test-stderr-agent").Run(context.Background(), Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  "test prompt",
	}, rec)

	// Verify that we get an error or non-zero exit code
	if err == nil && result.ExitCode == 0 {
		t.Error("Expected error or non-zero exit code for failed process")
	}

	// Verify that stderr was captured
	if !strings.Contains(result.Error, "error message") {
		t.Errorf("Expected stderr to contain 'error message', got: %q", result.Error)
	}
}
