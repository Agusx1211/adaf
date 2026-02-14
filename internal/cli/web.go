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
	"sort"
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
	webProjectsFile    = "web-projects.json"
	webMDNSServiceType = "_adaf._tcp"
)

type webRuntimeState struct {
	PID    int    `json:"pid"`
	URL    string `json:"url"`
	Port   int    `json:"port"`
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
}

type webProjectRecord struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type webProjectRegistryFile struct {
	Projects []webProjectRecord `json:"projects"`
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

var webRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register the current project for --registry web serving",
	Args:  cobra.NoArgs,
	RunE:  runWebRegister,
}

var webUnregisterCmd = &cobra.Command{
	Use:   "unregister",
	Short: "Unregister the current project from the web registry",
	Args:  cobra.NoArgs,
	RunE:  runWebUnregister,
}

var webListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects registered for web serving",
	Args:  cobra.NoArgs,
	RunE:  runWebList,
}

func init() {
	addWebServerFlags(webCmd, "open", "Open browser automatically", false)
	webCmd.AddCommand(webStopCmd, webStatusCmd, webRegisterCmd, webUnregisterCmd, webListCmd)
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
	cmd.Flags().StringSlice("projects", nil, "Comma-separated list of project directories to serve")
	cmd.Flags().Bool("multi", false, "Auto-discover projects in parent directory")
	cmd.Flags().Bool("registry", false, "Serve projects from ~/.adaf/web-projects.json")
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
	projects, _ := cmd.Flags().GetStringSlice("projects")
	multi, _ := cmd.Flags().GetBool("multi")
	useRegistry, _ := cmd.Flags().GetBool("registry")
	daemon, _ := cmd.Flags().GetBool("daemon")
	enableMDNS, _ := cmd.Flags().GetBool("mdns")
	daemonChild := os.Getenv(webDaemonChildEnv) == "1"
	useProjects := cmd.Flags().Changed("projects")
	userProvidedAuthToken := cmd.Flags().Changed("auth-token")

	if useProjects && multi {
		return fmt.Errorf("--projects and --multi cannot be used together")
	}
	if useRegistry && (useProjects || multi) {
		return fmt.Errorf("--registry cannot be used with --projects or --multi")
	}

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
	if daemonChild {
		state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
		if err != nil {
			return fmt.Errorf("checking existing web daemon: %w", err)
		}
		if running && state.PID != os.Getpid() {
			return fmt.Errorf("web server is already running (pid %d)", state.PID)
		}
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
	mdnsServiceName := "adaf"

	if useProjects {
		registry := webserver.NewProjectRegistry()
		currentDir, err := currentDirAbs()
		if err != nil {
			return err
		}
		entries := make([]webProjectRecord, 0, len(projects))
		for _, projectPath := range projects {
			entries = append(entries, webProjectRecord{Path: projectPath})
		}
		servedProjectIDs, err = registerProjectsIntoRegistry(registry, entries, currentDir, "--projects")
		if err != nil {
			return err
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
	} else if useRegistry {
		currentDir, err := currentDirAbs()
		if err != nil {
			return err
		}
		registryFile, err := loadWebProjectRegistry(webProjectsRegistryPath())
		if err != nil {
			return fmt.Errorf("loading web registry: %w", err)
		}
		if len(registryFile.Projects) == 0 {
			return fmt.Errorf("no registered projects found (run 'adaf web register')")
		}

		registry := webserver.NewProjectRegistry()
		servedProjectIDs, err = registerProjectsIntoRegistry(registry, registryFile.Projects, currentDir, "--registry")
		if err != nil {
			return err
		}
		srv = webserver.NewMulti(registry, opts)
	} else {
		s, err := openStoreRequired()
		if err != nil {
			return err
		}
		if cfg, err := s.LoadProject(); err == nil {
			if name := strings.TrimSpace(cfg.Name); name != "" {
				mdnsServiceName = name
			}
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

func runWebRegister(cmd *cobra.Command, args []string) error {
	project, err := currentWebProjectRecord()
	if err != nil {
		return err
	}

	registryPath := webProjectsRegistryPath()
	registry, err := loadWebProjectRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("loading web project registry: %w", err)
	}
	if !addWebProject(registry, project) {
		fmt.Fprintf(cmd.OutOrStdout(), "Project already registered: %s\n", project.Path)
		return nil
	}
	if err := saveWebProjectRegistry(registryPath, registry); err != nil {
		return fmt.Errorf("saving web project registry: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Registered project %q at %s\n", project.ID, project.Path)
	return nil
}

func runWebUnregister(cmd *cobra.Command, args []string) error {
	project, err := currentWebProjectRecord()
	if err != nil {
		return err
	}

	registryPath := webProjectsRegistryPath()
	registry, err := loadWebProjectRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("loading web project registry: %w", err)
	}
	if !removeWebProject(registry, project.Path) {
		fmt.Fprintf(cmd.OutOrStdout(), "Project not registered: %s\n", project.Path)
		return nil
	}
	if err := saveWebProjectRegistry(registryPath, registry); err != nil {
		return fmt.Errorf("saving web project registry: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Unregistered project %q (%s)\n", project.ID, project.Path)
	return nil
}

func runWebList(cmd *cobra.Command, args []string) error {
	registry, err := loadWebProjectRegistry(webProjectsRegistryPath())
	if err != nil {
		return fmt.Errorf("loading web project registry: %w", err)
	}
	if len(registry.Projects) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No registered web projects.")
		return nil
	}

	for _, project := range registry.Projects {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", project.ID, project.Path)
	}
	return nil
}

func registerProjectsIntoRegistry(registry *webserver.ProjectRegistry, projects []webProjectRecord, currentDir string, sourceLabel string) ([]string, error) {
	defaultID := ""
	for _, entry := range projects {
		projectPath := strings.TrimSpace(entry.Path)
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

		projectID := strings.TrimSpace(entry.ID)
		if projectID == "" {
			projectID = filepath.Base(absPath)
		}

		if err := registry.Register(projectID, absPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping project %q: %v\n", absPath, err)
			continue
		}
		if currentDir != "" && absPath == currentDir {
			defaultID = projectID
		}
	}

	if registry.Count() == 0 {
		if sourceLabel != "" {
			return nil, fmt.Errorf("no valid projects to serve from %s", sourceLabel)
		}
		return nil, fmt.Errorf("no valid projects to serve")
	}
	if defaultID != "" {
		if err := registry.SetDefault(defaultID); err != nil {
			return nil, fmt.Errorf("setting default project %q: %w", defaultID, err)
		}
	}

	entries := registry.List()
	servedProjectIDs := make([]string, 0, len(entries))
	for _, e := range entries {
		servedProjectIDs = append(servedProjectIDs, e.ID)
	}
	return servedProjectIDs, nil
}

func currentWebProjectRecord() (webProjectRecord, error) {
	s, err := openStoreRequired()
	if err != nil {
		return webProjectRecord{}, err
	}
	projectDir := filepath.Dir(s.Root())
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		return webProjectRecord{}, fmt.Errorf("resolving project path: %w", err)
	}
	absPath = filepath.Clean(absPath)
	return webProjectRecord{
		ID:   filepath.Base(absPath),
		Path: absPath,
	}, nil
}

func webPIDFilePath() string {
	return filepath.Join(config.Dir(), webPIDFileName)
}

func webStateFilePath() string {
	return filepath.Join(config.Dir(), webStateFileName)
}

func webProjectsRegistryPath() string {
	return filepath.Join(config.Dir(), webProjectsFile)
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

func loadWebProjectRegistry(path string) (*webProjectRegistryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &webProjectRegistryFile{Projects: []webProjectRecord{}}, nil
		}
		return nil, err
	}
	var registry webProjectRegistryFile
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, err
	}
	if registry.Projects == nil {
		registry.Projects = []webProjectRecord{}
	}
	return &registry, nil
}

func saveWebProjectRegistry(path string, registry *webProjectRegistryFile) error {
	if registry == nil {
		registry = &webProjectRegistryFile{}
	}
	if registry.Projects == nil {
		registry.Projects = []webProjectRecord{}
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func addWebProject(registry *webProjectRegistryFile, project webProjectRecord) bool {
	if registry == nil {
		return false
	}
	project.Path = filepath.Clean(strings.TrimSpace(project.Path))
	project.ID = strings.TrimSpace(project.ID)
	for _, existing := range registry.Projects {
		if filepath.Clean(existing.Path) == project.Path {
			return false
		}
	}
	registry.Projects = append(registry.Projects, project)
	sort.Slice(registry.Projects, func(i, j int) bool {
		if registry.Projects[i].ID == registry.Projects[j].ID {
			return registry.Projects[i].Path < registry.Projects[j].Path
		}
		return registry.Projects[i].ID < registry.Projects[j].ID
	})
	return true
}

func removeWebProject(registry *webProjectRegistryFile, projectPath string) bool {
	if registry == nil {
		return false
	}
	cleanedPath := filepath.Clean(strings.TrimSpace(projectPath))
	if cleanedPath == "" {
		return false
	}
	out := registry.Projects[:0]
	removed := false
	for _, project := range registry.Projects {
		if filepath.Clean(project.Path) == cleanedPath {
			removed = true
			continue
		}
		out = append(out, project)
	}
	registry.Projects = out
	return removed
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
