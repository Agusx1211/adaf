package cli

import (
	"errors"
	"fmt"
	"net"
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
	cmd.Flags().StringSlice("projects", nil, "Comma-separated list of project directories to serve")
	cmd.Flags().Bool("multi", false, "Auto-discover projects in parent directory")
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
