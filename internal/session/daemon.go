package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/loop"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

// StartDaemon launches a new daemon process for the given session ID.
// It waits for the daemon to create its socket (up to 10 seconds) before returning.
func StartDaemon(sessionID int) error {
	// Find our own executable path.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	cmd := exec.Command(exe, "_session-daemon", "--id", fmt.Sprintf("%d", sessionID))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Detach: we don't wait for the daemon to exit.
	go cmd.Wait()

	// Wait for the socket to appear.
	sockPath := SocketPath(sessionID)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("daemon did not create socket within 10 seconds")
}

// RunDaemon is the main entry point for the daemon process.
// It reads the session config, runs the agent loop, and serves events
// over a Unix domain socket.
func RunDaemon(sessionID int) error {
	cfg, err := LoadConfig(sessionID)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Update metadata with our PID.
	meta, err := LoadMeta(sessionID)
	if err != nil {
		return fmt.Errorf("loading meta: %w", err)
	}
	meta.PID = os.Getpid()
	meta.Status = "running"
	if err := SaveMeta(sessionID, meta); err != nil {
		return fmt.Errorf("saving meta: %w", err)
	}

	// Open the events file for replay logging.
	eventsFile, err := os.OpenFile(EventsPath(sessionID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening events file: %w", err)
	}
	defer eventsFile.Close()

	// Create the Unix socket.
	sockPath := SocketPath(sessionID)
	os.Remove(sockPath) // clean up any stale socket
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("creating socket: %w", err)
	}
	defer func() {
		listener.Close()
		os.Remove(sockPath)
	}()

	// Broadcaster manages connected clients and event distribution.
	b := &broadcaster{
		eventsFile: eventsFile,
		meta: WireMeta{
			SessionID:   sessionID,
			ProfileName: cfg.ProfileName,
			AgentName:   cfg.AgentName,
			ProjectName: cfg.ProjectName,
		},
	}

	// Context for the agent loop.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Accept client connections in background.
	go b.acceptLoop(listener, cancel)

	// Run the agent loop.
	loopErr := b.runAgentLoop(ctx, sessionID, cfg)

	// Update session metadata.
	meta, _ = LoadMeta(sessionID)
	if meta != nil {
		meta.EndedAt = time.Now().UTC()
		if loopErr != nil {
			meta.Status = "error"
			meta.Error = loopErr.Error()
		} else {
			meta.Status = "done"
		}
		SaveMeta(sessionID, meta)
	}

	// Wait a bit for clients to read final events, then shut down.
	b.waitForClients(30 * time.Second)

	return loopErr
}

// broadcaster manages client connections and event broadcasting.
type broadcaster struct {
	mu         sync.Mutex
	clients    []*clientConn
	buffered   [][]byte // all events for replay
	eventsFile *os.File
	meta       WireMeta
	done       bool
}

type clientConn struct {
	conn   net.Conn
	writer *bufio.Writer
	mu     sync.Mutex
}

func (b *broadcaster) acceptLoop(listener net.Listener, cancelAgent context.CancelFunc) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return // listener closed
		}
		go b.handleClient(conn, cancelAgent)
	}
}

func (b *broadcaster) handleClient(conn net.Conn, cancelAgent context.CancelFunc) {
	cc := &clientConn{
		conn:   conn,
		writer: bufio.NewWriter(conn),
	}

	// Send metadata.
	metaLine, _ := EncodeMsg(MsgMeta, b.meta)
	cc.writeLine(metaLine)

	// Send buffered events (replay).
	b.mu.Lock()
	for _, line := range b.buffered {
		cc.writeLine(line)
	}
	isDone := b.done
	b.clients = append(b.clients, cc)
	b.mu.Unlock()

	// Send live marker.
	liveLine, _ := EncodeMsg(MsgLive, nil)
	cc.writeLine(liveLine)
	cc.flush()

	if isDone {
		// Agent already finished; send done and close.
		doneLine, _ := EncodeMsg(MsgDone, nil)
		cc.writeLine(doneLine)
		cc.flush()
	}

	// Read control messages from the client.
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if line == CtrlCancel {
			cancelAgent()
		}
	}

	// Client disconnected.
	b.removeClient(cc)
	conn.Close()
}

func (b *broadcaster) removeClient(cc *clientConn) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, c := range b.clients {
		if c == cc {
			b.clients = append(b.clients[:i], b.clients[i+1:]...)
			return
		}
	}
}

// broadcast sends a message to all connected clients and buffers it for replay.
func (b *broadcaster) broadcast(line []byte) {
	b.mu.Lock()
	b.buffered = append(b.buffered, line)

	// Write to events file.
	b.eventsFile.Write(line)

	clients := make([]*clientConn, len(b.clients))
	copy(clients, b.clients)
	b.mu.Unlock()

	for _, cc := range clients {
		cc.writeLine(line)
		cc.flush()
	}
}

func (b *broadcaster) markDone() {
	b.mu.Lock()
	b.done = true
	clients := make([]*clientConn, len(b.clients))
	copy(clients, b.clients)
	b.mu.Unlock()

	doneLine, _ := EncodeMsg(MsgDone, nil)
	for _, cc := range clients {
		cc.writeLine(doneLine)
		cc.flush()
	}
}

func (b *broadcaster) waitForClients(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.clients)
		b.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (cc *clientConn) writeLine(data []byte) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.writer.Write(data)
}

func (cc *clientConn) flush() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.writer.Flush()
}

// runAgentLoop runs the agent and broadcasts events through the broadcaster.
func (b *broadcaster) runAgentLoop(ctx context.Context, sessionID int, cfg *DaemonConfig) error {
	// Open the project store.
	s, err := store.New(cfg.ProjectDir)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}

	// Look up the agent from the registry.
	agentInstance, ok := agent.Get(cfg.AgentName)
	if !ok {
		return fmt.Errorf("unknown agent %q", cfg.AgentName)
	}

	// Build agent config.
	streamCh := make(chan stream.RawEvent, 64)
	agentCfg := agent.Config{
		Name:      cfg.AgentName,
		Command:   cfg.AgentCommand,
		Args:      cfg.AgentArgs,
		Env:       cfg.AgentEnv,
		WorkDir:   cfg.WorkDir,
		Prompt:    cfg.Prompt,
		MaxTurns:  cfg.MaxTurns,
		EventSink: streamCh,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	}

	// Set ADAF_AGENT=1 so spawned agents can't use session commands.
	if agentCfg.Env == nil {
		agentCfg.Env = make(map[string]string)
	}
	agentCfg.Env["ADAF_AGENT"] = "1"

	var promptFunc func(sessionID int, supervisorNotes []store.SupervisorNote) string
	if cfg.UseDefaultPrompt {
		projCfg, err := s.LoadProject()
		if err == nil && projCfg != nil {
			globalCfg, _ := config.Load()
			var prof *config.Profile
			if globalCfg != nil && cfg.ProfileName != "" {
				prof = globalCfg.FindProfile(cfg.ProfileName)
			}
			basePrompt := agentCfg.Prompt
			promptFunc = func(sessionID int, supervisorNotes []store.SupervisorNote) string {
				built, err := promptpkg.Build(promptpkg.BuildOpts{
					Store:           s,
					Project:         projCfg,
					Profile:         prof,
					GlobalCfg:       globalCfg,
					SupervisorNotes: supervisorNotes,
				})
				if err != nil {
					return basePrompt
				}
				return built
			}
		}
	}

	// Bridge goroutine: converts stream events to wire messages.
	go func() {
		for ev := range streamCh {
			if ev.Err != nil {
				continue
			}
			if ev.Text != "" {
				wireRaw := WireRaw{
					Data:      ev.Text,
					SessionID: ev.SessionID,
				}
				line, err := EncodeMsg(MsgRaw, wireRaw)
				if err != nil {
					continue
				}
				b.broadcast(line)
				continue
			}
			eventJSON, err := json.Marshal(ev.Parsed)
			if err != nil {
				continue
			}
			wireEv := WireEvent{
				Event: eventJSON,
				Raw:   ev.Raw,
			}
			line, err := EncodeMsg(MsgEvent, wireEv)
			if err != nil {
				continue
			}
			b.broadcast(line)
		}
	}()

	// Run the loop.
	l := &loop.Loop{
		Store:      s,
		Agent:      agentInstance,
		Config:     agentCfg,
		PromptFunc: promptFunc,
		OnStart: func(sid int) {
			line, _ := EncodeMsg(MsgStarted, WireStarted{SessionID: sid})
			b.broadcast(line)
		},
		OnEnd: func(sid int, result *agent.Result) {
			wf := WireFinished{SessionID: sid}
			if result != nil {
				wf.ExitCode = result.ExitCode
				wf.DurationNS = result.Duration
			}
			line, _ := EncodeMsg(MsgFinished, wf)
			b.broadcast(line)
		},
	}

	loopErr := l.Run(ctx)
	close(streamCh)

	// Send done.
	wd := WireDone{}
	if loopErr != nil && loopErr != context.Canceled {
		wd.Error = loopErr.Error()
	}
	line, _ := EncodeMsg(MsgDone, wd)
	b.broadcast(line)
	b.markDone()

	return loopErr
}

// PopulateAgentRegistry ensures the agent registry has entries for the given
// agent. This is called by the daemon before running the loop.
func PopulateAgentRegistry(projectDir string) {
	agentsCfg, err := agent.LoadAgentsConfig(
		fmt.Sprintf("%s/.adaf", projectDir),
	)
	if err != nil {
		return
	}
	agent.PopulateFromConfig(agentsCfg)
}

// NewRecorder creates a recorder for the daemon's use. Exposed so the daemon
// command can set up recording if needed.
func NewRecorder(sessionID int, s *store.Store) *recording.Recorder {
	return recording.New(sessionID, s)
}
