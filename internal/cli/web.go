package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
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
	webCmd.Flags().StringSlice("projects", nil, "Comma-separated list of project directories to serve")
	webCmd.Flags().Bool("multi", false, "Auto-discover projects in parent directory")
	webCmd.Flags().Bool("open", false, "Open browser automatically")
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("web is not available inside an agent context")
	}

	port, _ := cmd.Flags().GetInt("port")
	host, _ := cmd.Flags().GetString("host")
	expose, _ := cmd.Flags().GetBool("expose")
	tlsMode, _ := cmd.Flags().GetString("tls")
	certFile, _ := cmd.Flags().GetString("cert")
	keyFile, _ := cmd.Flags().GetString("key")
	authToken, _ := cmd.Flags().GetString("auth-token")
	rateLimit, _ := cmd.Flags().GetFloat64("rate-limit")
	projects, _ := cmd.Flags().GetStringSlice("projects")
	multi, _ := cmd.Flags().GetBool("multi")
	useProjects := cmd.Flags().Changed("projects")

	if useProjects && multi {
		return fmt.Errorf("--projects and --multi cannot be used together")
	}

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

	opts := webserver.Options{
		Host:      host,
		Port:      port,
		TLSMode:   tlsMode,
		CertFile:  certFile,
		KeyFile:   keyFile,
		AuthToken: authToken,
		RateLimit: rateLimit,
	}

	var srv *webserver.Server
	var servedProjectIDs []string

	if useProjects {
		registry := webserver.NewProjectRegistry()
		currentDir, err := currentDirAbs()
		if err != nil {
			return err
		}

		defaultID := ""
		for _, rawPath := range projects {
			projectPath := strings.TrimSpace(rawPath)
			if projectPath == "" {
				continue
			}

			absPath, err := filepath.Abs(projectPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to resolve project path %q: %v\n", projectPath, err)
				continue
			}
			absPath = filepath.Clean(absPath)

			info, err := os.Stat(absPath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "Warning: project directory does not exist, skipping: %s\n", absPath)
				} else {
					fmt.Fprintf(os.Stderr, "Warning: cannot access project directory %q, skipping: %v\n", absPath, err)
				}
				continue
			}
			if !info.IsDir() {
				fmt.Fprintf(os.Stderr, "Warning: project path is not a directory, skipping: %s\n", absPath)
				continue
			}

			projectID := filepath.Base(absPath)
			if err := registry.Register(projectID, absPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: skipping project %q: %v\n", absPath, err)
				continue
			}
			if absPath == currentDir {
				defaultID = projectID
			}
		}

		if registry.Count() == 0 {
			return fmt.Errorf("no valid projects to serve from --projects")
		}
		if defaultID != "" {
			if err := registry.SetDefault(defaultID); err != nil {
				return fmt.Errorf("setting default project %q: %w", defaultID, err)
			}
		}

		entries := registry.List()
		servedProjectIDs = make([]string, 0, len(entries))
		for _, entry := range entries {
			servedProjectIDs = append(servedProjectIDs, entry.ID)
		}

		srv = webserver.NewMulti(registry, opts)
	} else if multi {
		currentDir, err := currentDirAbs()
		if err != nil {
			return err
		}

		registry := webserver.NewProjectRegistry()
		parentDir := filepath.Dir(currentDir)
		count, err := registry.ScanDirectory(parentDir)
		if err != nil {
			return fmt.Errorf("scanning parent directory %q for projects: %w", parentDir, err)
		}
		if count == 0 {
			return fmt.Errorf("no adaf projects found in parent directory %s", parentDir)
		}

		currentID := filepath.Base(currentDir)
		if _, ok := registry.Get(currentID); ok {
			if err := registry.SetDefault(currentID); err != nil {
				return fmt.Errorf("setting default project %q: %w", currentID, err)
			}
		}

		entries := registry.List()
		servedProjectIDs = make([]string, 0, len(entries))
		for _, entry := range entries {
			servedProjectIDs = append(servedProjectIDs, entry.ID)
		}

		srv = webserver.NewMulti(registry, opts)
	} else {
		s, err := openStoreRequired()
		if err != nil {
			return err
		}
		srv = webserver.New(s, opts)
	}

	if err := srv.Start(); err != nil {
		// Check for port-in-use error
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			fmt.Fprintf(os.Stderr, "Port %d is already in use.\n", port)
			fmt.Fprintf(os.Stderr, "Try: adaf web --port %d\n", port+1)
		}
		return fmt.Errorf("starting web server: %w", err)
	}

	// Build the full URL
	url := fmt.Sprintf("%s://%s", srv.Scheme(), srv.Addr())

	// Print clickable URL - use OSC 8 hyperlink escape sequences for terminals that support it
	fmt.Printf("\033]8;;%s\033\\%s\033]8;;\033\\\n", url, url)

	if len(servedProjectIDs) > 0 {
		label := "projects"
		if len(servedProjectIDs) == 1 {
			label = "project"
		}
		fmt.Printf("Serving %d %s: %s\n", len(servedProjectIDs), label, strings.Join(servedProjectIDs, ", "))
	}
	if authToken != "" {
		fmt.Printf("Auth token required for API access.\n")
	}

	// Open browser if --open flag is set
	open, _ := cmd.Flags().GetBool("open")
	if open {
		if err := openBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open browser: %v\n", err)
		}
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

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", url).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func currentDirAbs() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving working directory: %w", err)
	}
	return filepath.Clean(absDir), nil
}
