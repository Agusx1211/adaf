package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDaemonStartCommandFlags(t *testing.T) {
	cmd := &cobra.Command{}
	addWebServerFlags(cmd, "open-browser", "Open browser automatically", true)
	cmd.Flags().Bool("open", false, "Open browser automatically")

	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		t.Fatalf("getting port flag: %v", err)
	}
	if port != 8080 {
		t.Fatalf("port = %d, want 8080", port)
	}

	openBrowser, err := cmd.Flags().GetBool("open-browser")
	if err != nil {
		t.Fatalf("getting open-browser flag: %v", err)
	}
	if openBrowser {
		t.Fatalf("open-browser default = true, want false")
	}
}

func TestRunDaemonLifecycleStatusIncludesRegisteredProjectCount(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	state := webRuntimeState{
		PID:    os.Getpid(),
		URL:    "http://127.0.0.1:8181",
		Port:   8181,
		Host:   "127.0.0.1",
		Scheme: "http",
	}
	if err := writeWebRuntimeFiles(webPIDFilePath(), webStateFilePath(), state); err != nil {
		t.Fatalf("writing runtime files: %v", err)
	}

	registry := &webProjectRegistryFile{
		Projects: []webProjectRecord{
			{ID: "one", Path: filepath.Join(homeDir, "one")},
			{ID: "two", Path: filepath.Join(homeDir, "two")},
		},
	}
	if err := saveWebProjectRegistry(webProjectsRegistryPath(), registry); err != nil {
		t.Fatalf("saving registry: %v", err)
	}

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runDaemonLifecycleStatus(cmd, nil); err != nil {
		t.Fatalf("runDaemonLifecycleStatus() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Web daemon running (PID ") {
		t.Fatalf("status output missing pid line: %q", got)
	}
	if !strings.Contains(got, "URL: http://127.0.0.1:8181") {
		t.Fatalf("status output missing URL: %q", got)
	}
	if !strings.Contains(got, "Registered projects: 2") {
		t.Fatalf("status output missing registered project count: %q", got)
	}
}

func TestRunDaemonLifecycleStatusWhenNotRunning(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := runDaemonLifecycleStatus(cmd, nil); err != nil {
		t.Fatalf("runDaemonLifecycleStatus() error = %v", err)
	}

	if !strings.Contains(out.String(), "Web daemon not running.") {
		t.Fatalf("status output = %q, want not-running message", out.String())
	}
}
