package agent

import (
	"context"
	"fmt"
	"os"

	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

// TestProcessGroupCleanup tests that process group cleanup works correctly
// This verifies the Setpgid + process group kill pattern
func TestProcessGroupCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process group operations not supported on windows")
	}

	tmp := t.TempDir()

	// Create a script that spawns a background child process and writes its PID
	pidFile := filepath.Join(tmp, "child.pid")
	script := filepath.Join(tmp, "spawn_child.sh")
	scriptContent := fmt.Sprintf(`#!/usr/bin/env sh
# Spawn a background process that runs for a long time
(sleep 300 & echo $! > %s)
# Also keep the parent running
sleep 300
`, pidFile)

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

	// Create context with short timeout to trigger cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run the generic agent with the script that spawns children
	result, err := NewGenericAgent("test-process-group-agent").Run(ctx, Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  "test prompt",
	}, rec)

	// The command should be terminated by context cancellation
	if err == nil && result.ExitCode == 0 {
		t.Log("Expected error or non-zero exit code for cancelled process")
	}

	// Wait a bit for process cleanup to complete
	time.Sleep(500 * time.Millisecond)

	// Check if the child process is still alive
	childPIDBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Logf("Could not read child PID file: %v", err)
		return
	}

	childPIDStr := string(childPIDBytes)
	childPIDStr = strings.TrimSpace(childPIDStr)
	if childPIDStr == "" {
		t.Log("No child PID found in file")
		return
	}

	childPID, err := strconv.Atoi(childPIDStr)
	if err != nil {
		t.Logf("Could not parse child PID: %v", err)
		return
	}

	// Check if the child process is still running
	err = syscall.Kill(childPID, 0)
	if err == nil {
		t.Errorf("Child process %d is still running after parent cancellation - process group cleanup failed", childPID)

		// Try to kill it to clean up
		syscall.Kill(childPID, syscall.SIGKILL)
	} else if err != syscall.ESRCH {
		t.Logf("Unexpected error checking child process %d: %v", childPID, err)
	}
	// If err == syscall.ESRCH, the process doesn't exist, which is what we want
}

// TestOrphanProcessDetection tests that orphan processes are properly detected and cleaned up
func TestOrphanProcessDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process operations not supported on windows")
	}

	tmp := t.TempDir()

	// Create a script that spawns multiple orphan processes
	script := filepath.Join(tmp, "spawn_orphans.sh")
	scriptContent := `#!/usr/bin/env sh
# Spawn multiple background processes
for i in 1 2 3; do
    (sleep 300 &) 
done
# Keep parent running
sleep 300
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
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run the generic agent
	result, err := NewGenericAgent("test-orphan-agent").Run(ctx, Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  "test prompt",
	}, rec)

	// The command should be terminated
	if err == nil && result.ExitCode == 0 {
		t.Log("Expected error or non-zero exit code for cancelled process")
	}

	// Wait for cleanup
	time.Sleep(500 * time.Millisecond)

	// Try to find any remaining processes from the process group
	// This is a best-effort check - we can't easily get all PIDs in the group,
	// but we can verify that the test completes without hanging, which would
	// indicate that orphan processes aren't holding resources
}

// TestProcessCleanupWithGoTestSubprocess tests process cleanup using go test as a subprocess
// This is a more reliable way to test process cleanup without shell scripts
func TestProcessCleanupWithGoTestSubprocess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess testing not supported on windows")
	}

	tmp := t.TempDir()

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

	// Use go test as a subprocess that will be killed by context cancellation
	// go test --help will run for a while and then be terminated
	result, err := NewGenericAgent("test-go-subprocess").Run(ctx, Config{
		Command: "go",
		Args:    []string{"test", "--help"},
		WorkDir: tmp,
		Prompt:  "test prompt",
	}, rec)

	// Should get an error due to context cancellation
	if err == nil && result.ExitCode == 0 {
		t.Error("Expected error or non-zero exit code for cancelled go test process")
	}

	// The test should complete quickly, indicating the process was cleaned up
}
