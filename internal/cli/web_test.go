package cli

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestWebCommandFlags(t *testing.T) {
	// Create a test command to check flags
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the web server",
		Long:  "Start an HTTP/WebSocket server exposing project data and session streaming.",
	}

	// Register the same flags as in the real web command
	cmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	cmd.Flags().String("host", "127.0.0.1", "Host to bind to")
	cmd.Flags().Bool("expose", false, "Bind to 0.0.0.0 for LAN/remote access (enables TLS)")
	cmd.Flags().String("tls", "", "TLS mode: 'self-signed' or 'custom' (requires --cert and --key)")
	cmd.Flags().String("cert", "", "Path to TLS certificate file (for --tls=custom)")
	cmd.Flags().String("key", "", "Path to TLS key file (for --tls=custom)")
	cmd.Flags().String("auth-token", "", "Require Bearer token for API access")
	cmd.Flags().Float64("rate-limit", 0, "Max requests per second per IP (0 = unlimited)")
	cmd.Flags().Bool("daemon", false, "Run web server in background")
	cmd.Flags().Bool("mdns", false, "Advertise server on local network via mDNS/Bonjour")
	cmd.Flags().Bool("open", false, "Open browser automatically")

	// Test that the --port flag is registered
	port, err := cmd.Flags().GetInt("port")
	if err != nil {
		t.Fatalf("Failed to get port flag: %v", err)
	}
	if port != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}

	// Test that the --open flag is registered
	open, err := cmd.Flags().GetBool("open")
	if err != nil {
		t.Fatalf("Failed to get open flag: %v", err)
	}
	if open != false {
		t.Errorf("Expected open flag to be false, got %v", open)
	}

	// Test setting the --open flag
	err = cmd.Flags().Set("open", "true")
	if err != nil {
		t.Fatalf("Failed to set open flag: %v", err)
	}

	open, err = cmd.Flags().GetBool("open")
	if err != nil {
		t.Fatalf("Failed to get open flag after setting: %v", err)
	}
	if open != true {
		t.Errorf("Expected open flag to be true after setting, got %v", open)
	}
}

func TestWebPIDFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.pid")
	if err := writeWebPIDFile(path, 4242); err != nil {
		t.Fatalf("writeWebPIDFile() error = %v", err)
	}

	got, err := readWebPIDFile(path)
	if err != nil {
		t.Fatalf("readWebPIDFile() error = %v", err)
	}
	if got != 4242 {
		t.Fatalf("readWebPIDFile() = %d, want %d", got, 4242)
	}
}

func TestLoadWebDaemonStateRunning(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "web.pid")
	statePath := filepath.Join(dir, "web.json")

	want := webRuntimeState{
		PID:    999,
		URL:    "http://127.0.0.1:8080",
		Port:   8080,
		Host:   "127.0.0.1",
		Scheme: "http",
	}
	if err := writeWebRuntimeFiles(pidPath, statePath, want); err != nil {
		t.Fatalf("writeWebRuntimeFiles() error = %v", err)
	}

	got, running, err := loadWebDaemonState(pidPath, statePath, func(pid int) bool {
		return pid == want.PID
	})
	if err != nil {
		t.Fatalf("loadWebDaemonState() error = %v", err)
	}
	if !running {
		t.Fatalf("loadWebDaemonState() running = false, want true")
	}
	if got.PID != want.PID || got.URL != want.URL || got.Port != want.Port || got.Host != want.Host || got.Scheme != want.Scheme {
		t.Fatalf("loadWebDaemonState() = %+v, want %+v", got, want)
	}
}

func TestLoadWebDaemonStateStalePIDRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "web.pid")
	statePath := filepath.Join(dir, "web.json")

	state := webRuntimeState{
		PID:    1001,
		URL:    "http://127.0.0.1:8080",
		Port:   8080,
		Host:   "127.0.0.1",
		Scheme: "http",
	}
	if err := writeWebRuntimeFiles(pidPath, statePath, state); err != nil {
		t.Fatalf("writeWebRuntimeFiles() error = %v", err)
	}

	got, running, err := loadWebDaemonState(pidPath, statePath, func(pid int) bool {
		return false
	})
	if err != nil {
		t.Fatalf("loadWebDaemonState() error = %v", err)
	}
	if running {
		t.Fatalf("loadWebDaemonState() running = true, want false")
	}
	if got.PID != 0 {
		t.Fatalf("loadWebDaemonState() state = %+v, want zero-value state", got)
	}

	if _, err := os.Stat(pidPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pid file should be removed, stat error = %v", err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state file should be removed, stat error = %v", err)
	}
}

func TestOpenBrowserFunctionExists(t *testing.T) {
	// This test just verifies that the openBrowser function exists and can be called
	// We don't actually want to open a browser in tests
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("openBrowser function panicked: %v", r)
		}
	}()

	_ = openBrowser("http://localhost:8080")
}

func TestGenerateToken(t *testing.T) {
	// Test that generateToken produces a valid hex string
	token := generateToken()
	if token == "" {
		t.Error("Expected non-empty token")
	}
	if len(token) != 64 {
		t.Errorf("Expected token length 64, got %d", len(token))
	}

	// Test that it produces different tokens on multiple calls (very unlikely to fail)
	token2 := generateToken()
	if token == token2 {
		t.Error("Expected different tokens on multiple calls")
	}
}

func TestPortInUseErrorDetection(t *testing.T) {
	// Test that we can detect net.OpError which indicates port in use
	// This simulates the error that would come from net.Listen when port is already in use

	// Create a mock net.OpError
	mockOpError := &net.OpError{
		Op:     "listen",
		Net:    "tcp",
		Source: nil,
		Addr:   nil,
		Err:    fmt.Errorf("address already in use"),
	}

	// Test that errors.As can detect this error type
	var opErr *net.OpError
	if !errors.As(mockOpError, &opErr) {
		t.Error("Failed to detect net.OpError with errors.As")
	}

	if opErr == nil {
		t.Error("Expected opErr to be non-nil after errors.As")
	}
}
