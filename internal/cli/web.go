package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/webserver"
)

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the web server",
	Long:  `Start an HTTP/WebSocket server exposing project data and session streaming.`,
	RunE:  runWeb,
}

func init() {
	webCmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	webCmd.Flags().String("host", "127.0.0.1", "Host to bind to")
	webCmd.Flags().Bool("expose", false, "Bind to 0.0.0.0 for LAN/remote access (enables TLS)")
	webCmd.Flags().String("tls", "", "TLS mode: 'self-signed' or 'custom' (requires --cert and --key)")
	webCmd.Flags().String("cert", "", "Path to TLS certificate file (for --tls=custom)")
	webCmd.Flags().String("key", "", "Path to TLS key file (for --tls=custom)")
	webCmd.Flags().String("auth-token", "", "Require Bearer token for API access")
	webCmd.Flags().Float64("rate-limit", 0, "Max requests per second per IP (0 = unlimited)")
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("web is not available inside an agent context")
	}

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	expose, _ := cmd.Flags().GetBool("expose")
	tlsMode, _ := cmd.Flags().GetString("tls")
	certFile, _ := cmd.Flags().GetString("cert")
	keyFile, _ := cmd.Flags().GetString("key")
	authToken, _ := cmd.Flags().GetString("auth-token")
	rateLimit, _ := cmd.Flags().GetFloat64("rate-limit")

	if expose {
		host = "0.0.0.0"
		if !cmd.Flags().Changed("tls") {
			tlsMode = "self-signed"
		}
		if !cmd.Flags().Changed("auth-token") {
			authToken = generateToken()
			fmt.Fprintf(os.Stderr, "Generated auth token: %s\n", authToken)
		}
		fmt.Fprintln(os.Stderr, "Warning: Exposing web server on all interfaces.")
	}

	if tlsMode != "" && tlsMode != "self-signed" && tlsMode != "custom" {
		return fmt.Errorf("invalid --tls value %q, expected 'self-signed' or 'custom'", tlsMode)
	}
	if tlsMode == "custom" && (certFile == "" || keyFile == "") {
		return fmt.Errorf("--tls=custom requires both --cert and --key")
	}

	srv := webserver.New(s, webserver.Options{
		Host:      host,
		Port:      port,
		TLSMode:   tlsMode,
		CertFile:  certFile,
		KeyFile:   keyFile,
		AuthToken: authToken,
		RateLimit: rateLimit,
	})
	if err := srv.Start(); err != nil {
		return fmt.Errorf("starting web server: %w", err)
	}

	fmt.Printf("Web server running at %s://%s\n", srv.Scheme(), srv.Addr())
	if authToken != "" {
		fmt.Printf("Auth token required for API access.\n")
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down web server: %w", err)
	}

	return nil
}

func generateToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
