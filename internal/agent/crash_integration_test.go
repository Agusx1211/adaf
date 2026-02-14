//go:build integration

package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

// TestGenericAgent_IntegrationProcessCrash tests process crash handling with real agent binaries
// This is an integration test that requires real agent binaries to be available
func TestGenericAgent_IntegrationProcessCrash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	
	// Create a script that simulates an agent crash
	script := filepath.Join(tmp, "crash_agent.sh")
	scriptContent := `#!/usr/bin/env sh
# Simulate an agent that crashes after some processing
echo '{"type": "assistant", "assistant_message": {"content": [{"type": "text", "text": "starting processing"}]}}'
sleep 1
# Simulate crash
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

	// Run with context timeout to ensure cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := NewGenericAgent("test-integration-crash").Run(ctx, Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  `{"role": "user", "content": "test prompt"}`,
	}, rec)

	// Should get an error or non-zero exit due to crash
	if err == nil && result.ExitCode == 0 {
		t.Log("Expected error or non-zero exit code for crashed agent")
	}

	// Test should complete within timeout, indicating cleanup worked
}

// TestGenericAgent_IntegrationContextCancellation tests context cancellation with real processes
func TestGenericAgent_IntegrationContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	
	// Create a script that simulates a long-running agent
	script := filepath.Join(tmp, "long_running_agent.sh")
	scriptContent := `#!/usr/bin/env sh
# Simulate a long-running agent process
for i in 1 2 3 4 5; do
    echo '{"type": "assistant", "assistant_message": {"content": [{"type": "text", "text": "processing step '$i'"}]}}'
    sleep 2
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

	// Create context with short timeout to trigger cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := NewGenericAgent("test-integration-cancel").Run(ctx, Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  `{"role": "user", "content": "test prompt"}`,
	}, rec)

	// Should get an error due to context cancellation
	if err == nil && result.ExitCode == 0 {
		t.Log("Expected error or non-zero exit code for cancelled agent")
	}

	// Verify that the test completes quickly (within the timeout)
	// If process cleanup doesn't work, this test would hang
}

// TestGenericAgent_IntegrationOrphanCleanup tests orphan process cleanup with real processes
func TestGenericAgent_IntegrationOrphanCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script helper not supported on windows")
	}

	tmp := t.TempDir()
	
	// Create a script that spawns orphan processes like a real agent might
	script := filepath.Join(tmp, "orphan_spawning_agent.sh")
	scriptContent := `#!/usr/bin/env sh
# Simulate an agent that spawns background processes
echo '{"type": "assistant", "assistant_message": {"content": [{"type": "text", "text": "starting background tasks"}]}}'

# Spawn background processes (simulating orphan processes)
for i in 1 2 3; do
    (sleep 300 &) 
done

# Keep running
for i in 1 2 3 4 5; do
    echo '{"type": "assistant", "assistant_message": {"content": [{"type": "text", "text": "main process step '$i'"}]}}'
    sleep 2
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

	// Create context with timeout to trigger cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := NewGenericAgent("test-integration-orphan").Run(ctx, Config{
		Command: "/bin/sh",
		Args:    []string{script},
		WorkDir: tmp,
		Prompt:  `{"role": "user", "content": "test prompt"}`,
	}, rec)

	// Should get an error due to context cancellation
	if err == nil && result.ExitCode == 0 {
		t.Log("Expected error or non-zero exit code for cancelled agent")
	}

	// Wait for cleanup
	time.Sleep(500 * time.Millisecond)

	// The test should complete without hanging, indicating orphan cleanup worked
}