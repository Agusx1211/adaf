package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/mdns"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/webserver"
)

const (
	webDaemonChildEnv  = "ADAF_WEB_DAEMON_CHILD"
	webPIDFileName     = "web.pid"
	webStateFileName   = "web.json"
	webLockFileName    = "web.lock"
	webMDNSServiceType = "_adaf._tcp"
)

type webRuntimeState struct {
	PID    int    `json:"pid"`
	URL    string `json:"url"`
	Port   int    `json:"port"`
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
}

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start the web server",
	Long:  `Start an HTTP/WebSocket server exposing project data and session streaming.`,
	RunE:  runWeb,
}

var webStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemonized web server",
	Args:  cobra.NoArgs,
	RunE:  runWebStop,
}

var webStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemonized web server status",
	Args:  cobra.NoArgs,
	RunE:  runWebStatus,
}

func init() {
	addWebServerFlags(webCmd, "open", "Open browser automatically", false)
	webCmd.AddCommand(webStopCmd, webStatusCmd)
	rootCmd.AddCommand(webCmd)
}

func runWeb(cmd *cobra.Command, args []string) error {
	if os.Getenv(webDaemonChildEnv) != "1" && cmd.Flags().NFlag() == 0 {
		return runWebAsDaemonStart(cmd, args, true)
	}
	return runWebServe(cmd, args)
}

func addWebServerFlags(cmd *cobra.Command, openFlagName, openFlagUsage string, daemonDefault bool) {
	cmd.Flags().IntP("port", "p", 8080, "Port to listen on")
	cmd.Flags().String("host", "127.0.0.1", "Host to bind to")
	cmd.Flags().Bool("expose", false, "Bind to 0.0.0.0 for LAN/remote access (enables TLS)")
	cmd.Flags().String("tls", "", "TLS mode: 'self-signed' or 'custom' (requires --cert and --key)")
	cmd.Flags().String("cert", "", "Path to TLS certificate file (for --tls=custom)")
	cmd.Flags().String("key", "", "Path to TLS key file (for --tls=custom)")
	cmd.Flags().String("auth-token", "", "Require Bearer token for API access")
	cmd.Flags().Float64("rate-limit", 0, "Max requests per second per IP (0 = unlimited)")
	cmd.Flags().Bool("daemon", daemonDefault, "Run web server in background")
	cmd.Flags().Bool("mdns", false, "Advertise server on local network via mDNS/Bonjour")
	cmd.Flags().Bool(openFlagName, false, openFlagUsage)
}

func runWebServe(cmd *cobra.Command, args []string) error {
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
	daemon, _ := cmd.Flags().GetBool("daemon")
	enableMDNS, _ := cmd.Flags().GetBool("mdns")
	daemonChild := os.Getenv(webDaemonChildEnv) == "1"
	userProvidedAuthToken := cmd.Flags().Changed("auth-token")

	if expose {
		host = "0.0.0.0"
		if !cmd.Flags().Changed("tls") {
			tlsMode = "self-signed"
		}
		if !userProvidedAuthToken {
			authToken = generateToken()
			if !daemonChild {
				fmt.Fprintf(os.Stderr, "Generated auth token: %s\n", authToken)
			}
		}
		if !daemonChild {
			fmt.Fprintln(os.Stderr, "Warning: Exposing web server on all interfaces.")
		}
	}

	if tlsMode != "" && tlsMode != "self-signed" && tlsMode != "custom" {
		return fmt.Errorf("invalid --tls value %q, expected 'self-signed' or 'custom'", tlsMode)
	}
	if tlsMode == "custom" && (certFile == "" || keyFile == "") {
		return fmt.Errorf("--tls=custom requires both --cert and --key")
	}
	if daemon && !daemonChild {
		shouldInjectAuthToken := expose && !userProvidedAuthToken && strings.TrimSpace(authToken) != ""
		open, _ := cmd.Flags().GetBool("open")
		return runWebDaemonParent(authToken, shouldInjectAuthToken, expose, open)
	}
	// Acquire an exclusive file lock to prevent overlapping instances.
	lockFile, err := acquireWebDaemonLock()
	if err != nil {
		return fmt.Errorf("cannot start web server: %w", err)
	}
	defer lockFile.Close()

	opts := webserver.Options{
		Host:      host,
		Port:      port,
		TLSMode:   tlsMode,
		CertFile:  certFile,
		KeyFile:   keyFile,
		AuthToken: authToken,
		RateLimit: rateLimit,
	}

	rootDir, err := currentDirAbs()
	if err != nil {
		return err
	}
	opts.RootDir = rootDir
	mdnsServiceName := "adaf"

	registry := webserver.NewProjectRegistry()
	srv := webserver.NewMulti(registry, opts)

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
	hostPart, portPart := splitHostPort(srv.Addr())
	state := webRuntimeState{
		PID:    os.Getpid(),
		URL:    url,
		Port:   portPart,
		Host:   hostPart,
		Scheme: srv.Scheme(),
	}
	if daemonChild {
		if err := writeWebRuntimeFiles(webPIDFilePath(), webStateFilePath(), state); err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
			return fmt.Errorf("writing web daemon metadata: %w", err)
		}
		defer func() {
			_ = removeWebRuntimeFiles(webPIDFilePath(), webStateFilePath())
		}()
	}

	if !daemonChild {
		// Print clickable URL - use OSC 8 hyperlink escape sequences for terminals that support it
		fmt.Printf("\033]8;;%s\033\\%s\033]8;;\033\\\n", url, url)
		if expose {
			if err := printWebQRCode(url); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to render QR code: %v\n", err)
			}
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
	}

	shouldAdvertiseMDNS := expose || enableMDNS
	var mdnsServer *mdns.Server
	if shouldAdvertiseMDNS {
		server, err := startWebMDNSService(mdnsServiceName, state.Port, url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to start mDNS advertisement: %v\n", err)
		} else {
			mdnsServer = server
			defer mdnsServer.Shutdown()
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

func runWebDaemonParent(authToken string, injectAuthToken bool, printQR bool, openInBrowser bool) error {
	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil {
		return fmt.Errorf("checking existing web daemon: %w", err)
	}
	if running {
		return fmt.Errorf("web server is already running (pid %d)", state.PID)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	childArgs := daemonChildArgs(os.Args[1:])
	if injectAuthToken && authToken != "" && !hasAuthTokenArg(childArgs) {
		childArgs = append(childArgs, "--auth-token", authToken)
	}
	childCmd := exec.Command(exe, childArgs...)
	childCmd.Env = append(os.Environ(), webDaemonChildEnv+"=1")
	childCmd.Stdin = nil
	childCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := childCmd.Start(); err != nil {
		return fmt.Errorf("starting web daemon: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- childCmd.Wait()
	}()

	state, err = waitForWebDaemonStartup(waitCh, 8*time.Second)
	if err != nil {
		return err
	}

	fmt.Printf("Web server started in daemon mode.\n")
	fmt.Printf("URL: %s\n", state.URL)
	fmt.Printf("PID: %d\n", state.PID)
	if printQR {
		if err := printWebQRCode(state.URL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to render QR code: %v\n", err)
		}
	}
	if openInBrowser {
		if err := openBrowser(state.URL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open browser: %v\n", err)
		}
	}
	return nil
}

func waitForWebDaemonStartup(waitCh <-chan error, timeout time.Duration) (webRuntimeState, error) {
	deadline := time.Now().Add(timeout)
	for {
		state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
		if err != nil {
			return webRuntimeState{}, fmt.Errorf("reading web daemon state: %w", err)
		}
		if running && strings.TrimSpace(state.URL) != "" {
			return state, nil
		}

		select {
		case err := <-waitCh:
			if err == nil {
				return webRuntimeState{}, fmt.Errorf("web daemon exited before startup")
			}
			return webRuntimeState{}, fmt.Errorf("web daemon exited before startup: %w", err)
		default:
		}

		if time.Now().After(deadline) {
			return webRuntimeState{}, fmt.Errorf("timed out waiting for web daemon startup")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func runWebStop(cmd *cobra.Command, args []string) error {
	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil {
		return fmt.Errorf("checking web daemon status: %w", err)
	}
	if !running {
		fmt.Fprintln(cmd.OutOrStdout(), "No web server running.")
		return nil
	}

	if err := syscall.Kill(state.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("sending SIGTERM to web server pid %d: %w", state.PID, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isPIDAlive(state.PID) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if isPIDAlive(state.PID) {
		if err := syscall.Kill(state.PID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("sending SIGKILL to web server pid %d: %w", state.PID, err)
		}
	}

	if err := removeWebRuntimeFiles(webPIDFilePath(), webStateFilePath()); err != nil {
		return fmt.Errorf("removing web runtime metadata: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Web server stopped.")
	return nil
}

func runWebStatus(cmd *cobra.Command, args []string) error {
	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil {
		return fmt.Errorf("checking web daemon status: %w", err)
	}
	if !running {
		fmt.Fprintln(cmd.OutOrStdout(), "Web server not running.")
		return nil
	}

	url := strings.TrimSpace(state.URL)
	if url == "" && state.Port > 0 {
		scheme := strings.TrimSpace(state.Scheme)
		if scheme == "" {
			scheme = "http"
		}
		url = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(state.Host, strconv.Itoa(state.Port)))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Web server running (PID %d)\n", state.PID)
	if url != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "URL: %s\n", url)
	}
	return nil
}

func webPIDFilePath() string {
	return filepath.Join(config.Dir(), webPIDFileName)
}

func webStateFilePath() string {
	return filepath.Join(config.Dir(), webStateFileName)
}

func webLockFilePath() string {
	return filepath.Join(config.Dir(), webLockFileName)
}

// acquireWebDaemonLock takes an exclusive flock on the lock file, preventing
// two daemon instances from running concurrently. The returned file must be
// kept open for the lifetime of the daemon; closing it releases the lock.
func acquireWebDaemonLock() (*os.File, error) {
	if err := os.MkdirAll(config.Dir(), 0755); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}

	f, err := os.OpenFile(webLockFilePath(), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another web daemon is already running")
	}

	return f, nil
}

func writeWebRuntimeFiles(pidPath, statePath string, state webRuntimeState) error {
	if err := writeWebPIDFile(pidPath, state.PID); err != nil {
		return err
	}
	if err := writeWebRuntimeState(statePath, state); err != nil {
		_ = os.Remove(pidPath)
		return err
	}
	return nil
}

func removeWebRuntimeFiles(pidPath, statePath string) error {
	var errs []error
	if err := os.Remove(pidPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func loadWebDaemonState(pidPath, statePath string, pidAlive func(int) bool) (webRuntimeState, bool, error) {
	pid, err := readWebPIDFile(pidPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return webRuntimeState{}, false, nil
		}
		return webRuntimeState{}, false, err
	}

	if !pidAlive(pid) {
		_ = removeWebRuntimeFiles(pidPath, statePath)
		return webRuntimeState{}, false, nil
	}

	state, err := readWebRuntimeState(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return webRuntimeState{PID: pid}, true, nil
		}
		return webRuntimeState{}, false, err
	}

	state.PID = pid
	return state, true, nil
}

func writeWebPIDFile(path string, pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid: %d", pid)
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

func readWebPIDFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parsing pid file %s: %w", path, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid in %s", path)
	}
	return pid, nil
}

func writeWebRuntimeState(path string, state webRuntimeState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func readWebRuntimeState(path string) (webRuntimeState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return webRuntimeState{}, err
	}
	var state webRuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return webRuntimeState{}, err
	}
	return state, nil
}

func daemonChildArgs(args []string) []string {
	out := make([]string, 0, len(args))
	skipNext := false
	for i := range args {
		if skipNext {
			skipNext = false
			continue
		}
		arg := args[i]
		if arg == "--daemon" {
			if i+1 < len(args) {
				next := strings.ToLower(strings.TrimSpace(args[i+1]))
				if next == "true" || next == "false" || next == "1" || next == "0" {
					skipNext = true
				}
			}
			continue
		}
		if strings.HasPrefix(arg, "--daemon=") {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func hasAuthTokenArg(args []string) bool {
	for _, arg := range args {
		if arg == "--auth-token" || strings.HasPrefix(arg, "--auth-token=") {
			return true
		}
	}
	return false
}

func startWebMDNSService(projectName string, port int, url string) (*mdns.Server, error) {
	if port <= 0 {
		return nil, fmt.Errorf("invalid port for mDNS advertisement: %d", port)
	}
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "adaf"
	}
	txtRecords := []string{
		fmt.Sprintf("project=%s", name),
		fmt.Sprintf("url=%s", url),
	}
	service, err := mdns.NewMDNSService(name, webMDNSServiceType, "local", "", port, nil, txtRecords)
	if err != nil {
		return nil, err
	}
	return mdns.NewServer(&mdns.Config{
		Zone: service,
	})
}

func printWebQRCode(url string) error {
	code, err := qrcode.New(url, qrcode.Medium)
	if err != nil {
		return err
	}
	fmt.Println(code.ToString(false))
	return nil
}

func splitHostPort(addr string) (string, int) {
	host, rawPort, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return host, 0
	}
	return host, port
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
