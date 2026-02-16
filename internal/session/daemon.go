package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/events"
	"github.com/agusx1211/adaf/internal/looprun"
	"github.com/agusx1211/adaf/internal/orchestrator"
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
	cmd.Env = debug.PropagatedEnv(os.Environ(), fmt.Sprintf("session-daemon:%d", sessionID))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	logFile, err := os.OpenFile(DaemonLogPath(sessionID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening daemon log: %w", err)
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	// Wait for the socket to appear.
	sockPath := SocketPath(sessionID)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(sockPath); err == nil {
			return nil
		}
		select {
		case waitErr := <-waitCh:
			return buildDaemonStartupError(sessionID, "daemon exited before creating socket", waitErr)
		default:
		}
		if time.Now().After(deadline) {
			return buildDaemonStartupError(sessionID, "daemon did not create socket within 10 seconds", nil)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func buildDaemonStartupError(sessionID int, base string, waitErr error) error {
	logPath := DaemonLogPath(sessionID)
	lastLogLine := lastDaemonLogLine(logPath)
	message := fmt.Sprintf("%s (session #%d)", base, sessionID)
	if lastLogLine != "" {
		message += ": " + lastLogLine
	} else if waitErr != nil {
		message += fmt.Sprintf(": %v", waitErr)
	}
	message += fmt.Sprintf(" (daemon log: %s)", logPath)
	message += "; try 'adaf init' to repair missing project metadata directories"
	return errors.New(message)
}

func lastDaemonLogLine(path string) string {
	const maxReadBytes int64 = 32 * 1024

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return ""
	}

	start := stat.Size() - maxReadBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}

	data, err := io.ReadAll(f)
	if err != nil || len(data) == 0 {
		return ""
	}
	if start > 0 {
		if idx := bytes.IndexByte(data, '\n'); idx >= 0 && idx+1 < len(data) {
			data = data[idx+1:]
		}
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(stripANSI(lines[i]))
		if line == "" {
			continue
		}
		const maxLineLen = 320
		if len(line) > maxLineLen {
			line = line[:maxLineLen-3] + "..."
		}
		return line
	}
	return ""
}

func stripANSI(s string) string {
	var out strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// RunDaemon is the main entry point for the daemon process.
// It reads the session config, runs the agent loop, and serves events
// over a Unix domain socket.
func RunDaemon(sessionID int) error {
	debug.LogKV("session", "RunDaemon() starting", "session_id", sessionID, "pid", os.Getpid())
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
	meta.Status = StatusRunning
	if err := SaveMeta(sessionID, meta); err != nil {
		return fmt.Errorf("saving meta: %w", err)
	}
	_ = os.Setenv("ADAF_SESSION_ID", fmt.Sprintf("%d", sessionID))

	// Open the events file for diagnostics/forensics (`adaf log`).
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
			LoopName:    cfg.Loop.Name,
			LoopSteps:   len(cfg.Loop.Steps),
		},
		snapshot: WireSnapshot{
			Loop: WireSnapshotLoop{
				Profile:    cfg.ProfileName,
				TotalSteps: len(cfg.Loop.Steps),
			},
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

	// Set up HTTP server with WebSocket upgrade handler on the Unix socket.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		b.handleWSClient(w, r, cancel)
	})
	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			debug.LogKV("session", "HTTP server error", "session_id", sessionID, "error", err)
		}
	}()

	// Run the loop.
	loopErr := b.runLoop(ctx, cfg)

	// Update session metadata.
	meta, _ = LoadMeta(sessionID)
	if meta != nil {
		meta.EndedAt = time.Now().UTC()
		meta.Status, meta.Error = classifySessionEnd(loopErr)
		SaveMeta(sessionID, meta)
	}

	// Wait a bit for clients to read final events, then shut down.
	b.waitForClients(30 * time.Second)
	b.closeAllClients(websocket.StatusGoingAway, "daemon shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)

	return normalizeDaemonExit(loopErr)
}

// broadcaster manages client connections and event broadcasting.
type broadcaster struct {
	mu               sync.Mutex
	clients          []*clientConn
	streamSeq        int64
	eventsFile       *os.File
	meta             WireMeta
	done             bool
	snapshot         WireSnapshot
	snapshotRecentN  int
	lastModel        string
	lastLoopDone     *WireLoopDone
	lastDone         *WireDone
	eventsWriteError bool

	controlMu      sync.RWMutex
	controlHandler func(WireControl) WireControlResult
}

type clientConn struct {
	ws      *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	minSeq  int64
	closeMu sync.Once
}

type snapshotUpdate struct {
	model string
}

const (
	snapshotRecentEventLimit = 128
	snapshotRecentByteLimit  = 512 * 1024
	snapshotWireByteLimit    = 900 * 1024
	clientWriteTimeout       = 15 * time.Second
	clientPingInterval       = 30 * time.Second
	clientPingTimeout        = 15 * time.Second
	wireMsgTypeOverhead      = len(`{"type":""}`)
	wireMsgDataOverhead      = len(`,"data":`)
)

// handleWSClient upgrades an HTTP connection to WebSocket and serves a client.
func (b *broadcaster) handleWSClient(w http.ResponseWriter, r *http.Request, cancelAgent context.CancelFunc) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Unix socket, no origin check needed
	})
	if err != nil {
		debug.LogKV("session", "websocket accept failed", "session_id", b.meta.SessionID, "error", err)
		return
	}

	wsCtx, wsCancel := context.WithCancel(r.Context())
	cc := &clientConn{
		ws:     ws,
		ctx:    wsCtx,
		cancel: wsCancel,
	}
	defer func() {
		b.removeClient(cc)
	}()

	// Send metadata.
	metaLine, err := EncodeMsg(MsgMeta, b.meta)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session %d: encode meta failed: %v\n", b.meta.SessionID, err)
		return
	}
	if err := cc.writeImmediate(metaLine); err != nil {
		return
	}

	// Capture snapshot state under lock, but do NOT register the client yet.
	// This keeps snapshot and live-stream boundaries stable.
	b.mu.Lock()
	snapshot := cloneWireSnapshot(b.snapshot)
	loopDone := cloneLoopDone(b.lastLoopDone)
	donePayload := cloneDone(b.lastDone)
	done := b.done
	snapshotSeq := b.streamSeq
	b.mu.Unlock()

	snapshotLine, err := encodeBoundedSnapshot(snapshot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session %d: encode snapshot failed: %v\n", b.meta.SessionID, err)
		return
	}
	if err := cc.writeImmediate(snapshotLine); err != nil {
		return
	}

	// If the daemon already finished before this client connected, immediately
	// deliver terminal messages so the client can exit cleanly.
	if donePayload != nil || done {
		if loopDone != nil {
			line, err := EncodeMsg(MsgLoopDone, loopDone)
			if err != nil {
				fmt.Fprintf(os.Stderr, "session %d: encode loop_done failed: %v\n", b.meta.SessionID, err)
				return
			}
			if err := cc.writeImmediate(line); err != nil {
				return
			}
		}
		payload := WireDone{}
		if donePayload != nil {
			payload = *donePayload
		}
		line, err := EncodeMsg(MsgDone, payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session %d: encode done failed: %v\n", b.meta.SessionID, err)
			return
		}
		if err := cc.writeImmediate(line); err != nil {
			return
		}
		return
	}

	// Send live marker after snapshot for active sessions.
	liveLine, err := EncodeMsg(MsgLive, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session %d: encode live failed: %v\n", b.meta.SessionID, err)
		return
	}
	if err := cc.writeImmediate(liveLine); err != nil {
		return
	}

	b.mu.Lock()
	cc.minSeq = snapshotSeq + 1
	b.clients = append(b.clients, cc)
	b.mu.Unlock()
	b.startPingLoop(cc)

	// Read control messages from the client via WebSocket.
	for {
		_, data, err := ws.Read(wsCtx)
		if err != nil {
			return // client disconnected or context cancelled
		}
		line := string(data)
		if line == CtrlCancel {
			cancelAgent()
			continue
		}

		msg, err := DecodeMsg(data)
		if err != nil || msg.Type != MsgControl {
			continue
		}

		req, err := DecodeData[WireControl](msg)
		resp := WireControlResult{
			Action: "unknown",
			OK:     false,
		}
		if err != nil {
			resp.Error = fmt.Sprintf("invalid control payload: %v", err)
		} else {
			resp = b.runControl(*req)
		}
		respLine, err := EncodeMsg(MsgControlResult, resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "session %d: encode control_result failed: %v\n", b.meta.SessionID, err)
			continue
		}
		if err := cc.writeImmediate(respLine); err != nil {
			return
		}
	}
}

func (b *broadcaster) setControlHandler(h func(WireControl) WireControlResult) {
	b.controlMu.Lock()
	b.controlHandler = h
	b.controlMu.Unlock()
}

func (b *broadcaster) runControl(req WireControl) WireControlResult {
	b.controlMu.RLock()
	h := b.controlHandler
	b.controlMu.RUnlock()

	if h == nil {
		return WireControlResult{
			Action: req.Action,
			OK:     false,
			Error:  "daemon control handler is not ready",
		}
	}
	return h(req)
}

func (b *broadcaster) removeClient(cc *clientConn) {
	b.mu.Lock()
	for i, c := range b.clients {
		if c == cc {
			b.clients = append(b.clients[:i], b.clients[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
	cc.close(websocket.StatusNormalClosure, "")
}

// broadcast sends a pre-encoded message to all connected clients and records a
// compact reconnect snapshot (state + bounded recent output). This path is kept
// for tests and compatibility.
func (b *broadcaster) broadcast(line []byte) {
	lineCopy := append([]byte(nil), line...)
	msg, err := DecodeMsg(lineCopy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session %d: broadcast decode failed: %v\n", b.meta.SessionID, err)
	}
	b.broadcastPrepared(lineCopy, msg, nil, snapshotUpdate{})
}

// broadcastTyped sends a typed message to clients while updating snapshot state
// without re-decoding the encoded line on the hot path.
func (b *broadcaster) broadcastTyped(msgType string, payload any) {
	b.broadcastTypedWithUpdate(msgType, payload, snapshotUpdate{})
}

func (b *broadcaster) broadcastTypedWithUpdate(msgType string, payload any, update snapshotUpdate) {
	line, msg, err := encodeWireForBroadcast(msgType, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "session %d: encode %s failed: %v\n", b.meta.SessionID, msgType, err)
		return
	}
	b.broadcastPrepared(line, msg, payload, update)
}

func (b *broadcaster) broadcastPrepared(line []byte, msg *WireMsg, payload any, update snapshotUpdate) {
	b.mu.Lock()
	seq := b.streamSeq + 1
	b.streamSeq = seq
	if update.model != "" {
		b.lastModel = update.model
		if b.snapshot.Session != nil {
			b.snapshot.Session.Model = update.model
		}
	}
	if msg != nil {
		b.updateSnapshotLocked(msg, payload)
	}

	// Write to events file.
	if _, err := b.eventsFile.Write(line); err != nil && !b.eventsWriteError {
		b.eventsWriteError = true
		fmt.Fprintf(os.Stderr, "session %d: failed to append to events file: %v\n", b.meta.SessionID, err)
	}
	type queuedClient struct {
		conn   *clientConn
		minSeq int64
	}
	clients := make([]queuedClient, len(b.clients))
	for i, cc := range b.clients {
		clients[i] = queuedClient{conn: cc, minSeq: cc.minSeq}
	}
	b.mu.Unlock()

	for _, cc := range clients {
		if seq < cc.minSeq {
			continue
		}
		// A copied client entry may race with concurrent removal/close; write
		// failure means the client is gone or unhealthy and should be removed.
		if err := cc.conn.writeImmediate(line); err != nil {
			fmt.Fprintf(os.Stderr, "session %d: removing websocket client after write failure: %v\n", b.meta.SessionID, err)
			b.removeClient(cc.conn)
		}
	}
}

func (b *broadcaster) updateSnapshotLocked(msg *WireMsg, payload any) {
	switch msg.Type {
	case MsgStarted:
		data, ok := decodeWireData[WireStarted](msg, payload)
		if !ok || data.SessionID <= 0 {
			return
		}
		now := time.Now().UTC()
		action := "starting"
		startedAt := now
		resumed := false
		model := b.lastModel
		turnHexID := data.TurnHexID
		inputTokens := 0
		outputTokens := 0
		costUSD := 0.0
		numTurns := 0
		if b.snapshot.Session != nil {
			if b.snapshot.Session.Model != "" {
				model = b.snapshot.Session.Model
			}
			if b.snapshot.Session.SessionID == data.SessionID {
				if turnHexID == "" {
					turnHexID = b.snapshot.Session.TurnHexID
				}
				inputTokens = b.snapshot.Session.InputTokens
				outputTokens = b.snapshot.Session.OutputTokens
				costUSD = b.snapshot.Session.CostUSD
				numTurns = b.snapshot.Session.NumTurns
			}
			if b.snapshot.Session.SessionID == data.SessionID && b.snapshot.Session.Status == "waiting_for_spawns" {
				action = "resumed"
				resumed = true
				if !b.snapshot.Session.StartedAt.IsZero() {
					startedAt = b.snapshot.Session.StartedAt
				}
			}
		}
		b.snapshot.Session = &WireSnapshotSession{
			SessionID:    data.SessionID,
			TurnHexID:    turnHexID,
			Agent:        b.meta.AgentName,
			Profile:      b.snapshot.Loop.Profile,
			Model:        model,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			CostUSD:      costUSD,
			NumTurns:     numTurns,
			Status:       "running",
			Action:       action,
			StartedAt:    startedAt,
		}
		if !resumed {
			b.clearSnapshotRecentLocked()
		}

	case MsgPrompt:
		data, ok := decodeWireData[WirePrompt](msg, payload)
		if !ok {
			return
		}
		b.appendSnapshotRecentLocked(*msg)
		if b.snapshot.Session != nil && data.SessionID > 0 && b.snapshot.Session.SessionID == data.SessionID {
			b.snapshot.Session.TurnHexID = data.TurnHexID
			b.snapshot.Session.Action = "prompt ready"
		}

	case MsgEvent:
		data, ok := decodeWireData[WireEvent](msg, payload)
		if !ok {
			return
		}
		b.appendSnapshotRecentLocked(*msg)
		if b.snapshot.Session != nil {
			b.snapshot.Session.Action = "responding"
		}
		var ev stream.ClaudeEvent
		if err := json.Unmarshal(data.Event, &ev); err != nil {
			return
		}
		if ev.Model != "" {
			b.lastModel = ev.Model
			if b.snapshot.Session != nil {
				b.snapshot.Session.Model = ev.Model
			}
		}
		if ev.Type == "result" && b.snapshot.Session != nil {
			b.snapshot.Session.Action = "turn complete"
			if ev.TotalCostUSD > 0 {
				b.snapshot.Session.CostUSD = ev.TotalCostUSD
			}
			if ev.NumTurns > 0 {
				b.snapshot.Session.NumTurns = ev.NumTurns
			}
			if ev.Usage != nil {
				b.snapshot.Session.InputTokens = ev.Usage.InputTokens
				b.snapshot.Session.OutputTokens = ev.Usage.OutputTokens
			}
		}

	case MsgRaw:
		data, ok := decodeWireData[WireRaw](msg, payload)
		if !ok {
			return
		}
		b.appendSnapshotRecentLocked(*msg)
		if data.SessionID > 0 && (b.snapshot.Session == nil || b.snapshot.Session.SessionID != data.SessionID) {
			b.snapshot.Session = &WireSnapshotSession{
				SessionID: data.SessionID,
				Agent:     b.meta.AgentName,
				Profile:   b.snapshot.Loop.Profile,
				Model:     b.lastModel,
				Status:    "running",
				Action:    "responding",
				StartedAt: time.Now().UTC(),
			}
		} else if data.SessionID > 0 && b.snapshot.Session != nil {
			b.snapshot.Session.Action = "responding"
		}

	case MsgFinished:
		data, ok := decodeWireData[WireFinished](msg, payload)
		if !ok {
			return
		}
		if b.snapshot.Session == nil || b.snapshot.Session.SessionID != data.SessionID {
			if data.SessionID <= 0 {
				return
			}
			b.snapshot.Session = &WireSnapshotSession{
				SessionID: data.SessionID,
				Agent:     b.meta.AgentName,
				Profile:   b.snapshot.Loop.Profile,
				Model:     b.lastModel,
			}
		}
		if data.TurnHexID != "" {
			b.snapshot.Session.TurnHexID = data.TurnHexID
		}
		b.snapshot.Session.EndedAt = time.Now().UTC()
		switch {
		case data.Error != "":
			b.snapshot.Session.Status = "failed"
			b.snapshot.Session.Action = "error"
		case data.WaitForSpawns:
			b.snapshot.Session.Status = "waiting_for_spawns"
			b.snapshot.Session.Action = "waiting for spawns"
		case data.ExitCode == 0:
			b.snapshot.Session.Status = "completed"
			b.snapshot.Session.Action = "finished"
		default:
			b.snapshot.Session.Status = "failed"
			b.snapshot.Session.Action = "finished"
		}

	case MsgSpawn:
		data, ok := decodeWireData[WireSpawn](msg, payload)
		if !ok {
			return
		}
		if len(data.Spawns) == 0 {
			b.snapshot.Spawns = nil
		} else {
			spawns := make([]WireSpawnInfo, len(data.Spawns))
			copy(spawns, data.Spawns)
			b.snapshot.Spawns = spawns
		}

	case MsgLoopStepStart:
		data, ok := decodeWireData[WireLoopStepStart](msg, payload)
		if !ok {
			return
		}
		totalSteps := b.snapshot.Loop.TotalSteps
		if data.TotalSteps > 0 {
			totalSteps = data.TotalSteps
		}
		b.snapshot.Loop = WireSnapshotLoop{
			RunID:      data.RunID,
			RunHexID:   data.RunHexID,
			StepHexID:  data.StepHexID,
			Cycle:      data.Cycle,
			StepIndex:  data.StepIndex,
			Profile:    data.Profile,
			TotalSteps: totalSteps,
		}
		b.clearSnapshotRecentLocked()
		if b.snapshot.Session != nil && b.snapshot.Session.Profile == "" {
			b.snapshot.Session.Profile = data.Profile
		}

	case MsgLoopStepEnd:
		data, ok := decodeWireData[WireLoopStepEnd](msg, payload)
		if !ok {
			return
		}
		b.snapshot.Loop.RunID = data.RunID
		b.snapshot.Loop.RunHexID = data.RunHexID
		b.snapshot.Loop.StepHexID = data.StepHexID
		b.snapshot.Loop.Cycle = data.Cycle
		b.snapshot.Loop.StepIndex = data.StepIndex
		if data.Profile != "" {
			b.snapshot.Loop.Profile = data.Profile
		}
		if data.TotalSteps > 0 {
			b.snapshot.Loop.TotalSteps = data.TotalSteps
		}

	case MsgLoopDone:
		data, ok := decodeWireData[WireLoopDone](msg, payload)
		if !ok {
			return
		}
		cp := data
		b.lastLoopDone = &cp

	case MsgDone:
		data, ok := decodeWireData[WireDone](msg, payload)
		if !ok {
			// Done signal is authoritative even if payload is malformed.
			data = WireDone{}
		}
		cp := data
		b.lastDone = &cp
		b.done = true
	}
}

func (b *broadcaster) appendSnapshotRecentLocked(msg WireMsg) {
	switch msg.Type {
	case MsgPrompt, MsgEvent, MsgRaw:
	default:
		return
	}

	item := WireMsg{
		Type: msg.Type,
		Data: append(json.RawMessage(nil), msg.Data...),
	}
	b.snapshot.Recent = append(b.snapshot.Recent, item)
	b.snapshotRecentN += wireMsgSize(item)
	b.trimSnapshotRecentLocked()
}

func (b *broadcaster) trimSnapshotRecentLocked() {
	recent := b.snapshot.Recent
	if len(recent) == 0 {
		b.snapshotRecentN = 0
		return
	}

	drop := 0
	remaining := b.snapshotRecentN
	for i := 0; i < len(recent); i++ {
		needsCountDrop := len(recent)-i > snapshotRecentEventLimit
		needsByteDrop := remaining > snapshotRecentByteLimit
		if !needsCountDrop && !needsByteDrop {
			break
		}
		remaining -= wireMsgSize(recent[i])
		if remaining < 0 {
			remaining = 0
		}
		drop = i + 1
	}
	if drop == 0 {
		return
	}

	trimmed := make([]WireMsg, len(recent)-drop)
	copy(trimmed, recent[drop:])
	b.snapshot.Recent = trimmed
	b.snapshotRecentN = remaining
}

func (b *broadcaster) clearSnapshotRecentLocked() {
	// Preserve the most recent prompt so reconnecting clients see the
	// active turn's prompt in the detail view.
	var lastPrompt *WireMsg
	for i := len(b.snapshot.Recent) - 1; i >= 0; i-- {
		if b.snapshot.Recent[i].Type == MsgPrompt {
			cp := b.snapshot.Recent[i]
			lastPrompt = &cp
			break
		}
	}
	b.snapshot.Recent = nil
	b.snapshotRecentN = 0
	if lastPrompt != nil {
		b.snapshot.Recent = []WireMsg{*lastPrompt}
		b.snapshotRecentN = wireMsgSize(*lastPrompt)
	}
}

func decodeWireData[T any](msg *WireMsg, payload any) (T, bool) {
	var zero T
	if payload != nil {
		switch v := payload.(type) {
		case T:
			return v, true
		}
	}
	data, err := DecodeData[T](msg)
	if err != nil || data == nil {
		return zero, false
	}
	return *data, true
}

func wireMsgSize(msg WireMsg) int {
	n := wireMsgTypeOverhead + len(msg.Type)
	if len(msg.Data) > 0 {
		n += wireMsgDataOverhead + len(msg.Data)
	}
	return n
}

func encodeWireForBroadcast(msgType string, payload any) ([]byte, *WireMsg, error) {
	var data json.RawMessage
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, nil, err
		}
		data = raw
	}
	msg := &WireMsg{
		Type: msgType,
		Data: data,
	}
	line, err := json.Marshal(msg)
	if err != nil {
		return nil, nil, err
	}
	return append(line, '\n'), msg, nil
}

func encodeBoundedSnapshot(snapshot WireSnapshot) ([]byte, error) {
	snap := snapshot
	if snapshot.Session != nil {
		session := *snapshot.Session
		snap.Session = &session
	}

	line, err := EncodeMsg(MsgSnapshot, snap)
	if err != nil {
		return nil, err
	}
	if len(line) <= snapshotWireByteLimit {
		return line, nil
	}

	if len(snap.Recent) > 0 {
		// Recent is capped to 128 entries, so a linear trim keeps logic simple and
		// bounded while preserving as much tail output as possible.
		for len(snap.Recent) > 0 {
			snap.Recent = snap.Recent[1:]
			line, err = EncodeMsg(MsgSnapshot, snap)
			if err != nil {
				return nil, err
			}
			if len(line) <= snapshotWireByteLimit {
				return line, nil
			}
		}
	}

	if snap.Session != nil {
		snap.Session.Action = truncateSnapshotField(snap.Session.Action, 256)
		snap.Session.Model = truncateSnapshotField(snap.Session.Model, 128)
		snap.Session.Profile = truncateSnapshotField(snap.Session.Profile, 128)
		snap.Session.TurnHexID = truncateSnapshotField(snap.Session.TurnHexID, 64)
		line, err = EncodeMsg(MsgSnapshot, snap)
		if err != nil {
			return nil, err
		}
		if len(line) <= snapshotWireByteLimit {
			return line, nil
		}
	}

	if len(snap.Spawns) > 0 {
		snap.Spawns = nil
		line, err = EncodeMsg(MsgSnapshot, snap)
		if err != nil {
			return nil, err
		}
		if len(line) <= snapshotWireByteLimit {
			return line, nil
		}
	}

	line, err = EncodeMsg(MsgSnapshot, snap)
	if err != nil {
		return nil, err
	}
	if len(line) > snapshotWireByteLimit {
		return nil, fmt.Errorf("snapshot exceeds %d bytes even after trimming (%d)", snapshotWireByteLimit, len(line))
	}
	return line, nil
}

func truncateSnapshotField(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func cloneWireSnapshot(src WireSnapshot) WireSnapshot {
	dst := WireSnapshot{
		Loop: src.Loop,
	}
	if src.Session != nil {
		cp := *src.Session
		dst.Session = &cp
	}
	if len(src.Spawns) > 0 {
		dst.Spawns = make([]WireSpawnInfo, len(src.Spawns))
		copy(dst.Spawns, src.Spawns)
	}
	if len(src.Recent) > 0 {
		dst.Recent = make([]WireMsg, len(src.Recent))
		for i, msg := range src.Recent {
			dst.Recent[i] = WireMsg{
				Type: msg.Type,
				Data: append(json.RawMessage(nil), msg.Data...),
			}
		}
	}
	return dst
}

func cloneLoopDone(src *WireLoopDone) *WireLoopDone {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func cloneDone(src *WireDone) *WireDone {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
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

func (b *broadcaster) closeAllClients(status websocket.StatusCode, reason string) {
	b.mu.Lock()
	clients := append([]*clientConn(nil), b.clients...)
	b.clients = nil
	b.mu.Unlock()

	for _, cc := range clients {
		cc.close(status, reason)
	}
}

func (b *broadcaster) startPingLoop(cc *clientConn) {
	go func() {
		ticker := time.NewTicker(clientPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-cc.ctx.Done():
				return
			case <-ticker.C:
			}

			pingCtx, pingCancel := context.WithTimeout(cc.ctx, clientPingTimeout)
			err := cc.ws.Ping(pingCtx)
			pingCancel()
			if err != nil {
				b.removeClient(cc)
				return
			}
		}
	}()
}

func (cc *clientConn) close(status websocket.StatusCode, reason string) {
	cc.closeMu.Do(func() {
		cc.cancel()
		if cc.ws != nil {
			_ = cc.ws.Close(status, reason)
		}
	})
}

func (cc *clientConn) writeImmediate(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	// Strip trailing newline — WebSocket frames don't need line delimiters.
	msg := data
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	writeCtx, writeCancel := context.WithTimeout(cc.ctx, clientWriteTimeout)
	defer writeCancel()
	return cc.ws.Write(writeCtx, websocket.MessageText, msg)
}

// runLoop runs the loop runtime and broadcasts events through the broadcaster.
func (b *broadcaster) runLoop(ctx context.Context, cfg *DaemonConfig) error {
	debug.LogKV("session", "runLoop() starting",
		"session_id", b.meta.SessionID,
		"project_dir", cfg.ProjectDir,
		"workdir", cfg.WorkDir,
		"loop_name", cfg.Loop.Name,
		"loop_steps", len(cfg.Loop.Steps),
		"max_cycles", cfg.MaxCycles,
	)
	s, err := store.New(cfg.ProjectDir)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	if err := s.EnsureDirs(); err != nil {
		return fmt.Errorf("ensuring store dirs: %w", err)
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: append([]config.Profile(nil), cfg.Profiles...),
		Pushover: cfg.Pushover,
	}
	if loaded, err := config.Load(); err == nil && loaded != nil {
		// Merge any profiles from disk that are missing in the snapshot.
		// The snapshot should already include all needed profiles, but
		// merging from disk acts as a safety net (e.g. for delegation
		// profiles that a caller might have missed).
		existing := make(map[string]struct{}, len(globalCfg.Profiles))
		for _, p := range globalCfg.Profiles {
			existing[strings.ToLower(p.Name)] = struct{}{}
		}
		for _, p := range loaded.Profiles {
			if _, ok := existing[strings.ToLower(p.Name)]; !ok {
				globalCfg.Profiles = append(globalCfg.Profiles, p)
			}
		}
		if globalCfg.Pushover.UserKey == "" && globalCfg.Pushover.AppToken == "" {
			globalCfg.Pushover = loaded.Pushover
		}
	}

	agentsCfg, err := agent.LoadAgentsConfig()
	if err != nil {
		return fmt.Errorf("loading agent config: %w", err)
	}
	applyAgentCommandOverrides(agentsCfg, cfg.AgentCommandOverrides)
	agent.PopulateFromConfig(agentsCfg)

	loopDef := cfg.Loop
	if loopDef.Name == "" {
		loopDef.Name = "session-loop"
	}
	if len(loopDef.Steps) == 0 {
		return fmt.Errorf("loop %q has no steps", loopDef.Name)
	}
	workDir := cfg.WorkDir
	if workDir == "" && projCfg != nil {
		workDir = projCfg.RepoPath
	}

	orch := orchestrator.Init(s, globalCfg, workDir)
	b.setControlHandler(func(req WireControl) WireControlResult {
		resp := WireControlResult{
			Action: req.Action,
			OK:     false,
		}

		switch req.Action {
		case "spawn":
			if req.Spawn == nil {
				resp.Error = "missing spawn request payload"
				return resp
			}

			spawnReq := orchestrator.SpawnRequest{
				ParentTurnID:  req.Spawn.ParentTurnID,
				ParentProfile: req.Spawn.ParentProfile,
				ChildProfile:  req.Spawn.ChildProfile,
				ChildRole:     req.Spawn.ChildRole,
				PlanID:        req.Spawn.PlanID,
				Task:          req.Spawn.Task,
				IssueIDs:      req.Spawn.IssueIDs,
				ReadOnly:      req.Spawn.ReadOnly,
				Wait:          req.Spawn.Wait,
				Delegation:    req.Spawn.Delegation,
			}

			spawnID, err := orch.Spawn(ctx, spawnReq)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			resp.OK = true
			resp.SpawnID = spawnID
			if req.Spawn.Wait {
				result := orch.WaitOne(spawnID)
				resp.Status = result.Status
				resp.ExitCode = result.ExitCode
				resp.Result = result.Result
			}
			return resp
		case "wait":
			if req.Wait == nil {
				resp.Error = "missing wait request payload"
				return resp
			}
			if req.Wait.TurnID <= 0 {
				resp.Error = "wait request turn_id must be > 0"
				return resp
			}
			if err := s.SignalWait(req.Wait.TurnID); err != nil {
				resp.Error = err.Error()
				return resp
			}
			resp.OK = true
			return resp
		case "interrupt_spawn":
			if req.Interrupt == nil {
				resp.Error = "missing interrupt request payload"
				return resp
			}
			if req.Interrupt.SpawnID <= 0 {
				resp.Error = "interrupt request spawn_id must be > 0"
				return resp
			}
			if err := orch.InterruptSpawn(req.Interrupt.SpawnID, req.Interrupt.Message); err != nil {
				resp.Error = err.Error()
				return resp
			}
			resp.OK = true
			return resp
		default:
			resp.Error = fmt.Sprintf("unsupported control action %q", req.Action)
			return resp
		}
	})
	defer b.setControlHandler(nil)

	eventCh := make(chan any, 256)
	orch.SetEventCh(eventCh)
	forwardDone := make(chan struct{})
	loopRunID := 0
	loopRunHexID := ""
	go func() {
		defer close(forwardDone)
		totalSteps := len(loopDef.Steps)
		for msg := range eventCh {
			switch ev := msg.(type) {
			case events.AgentStartedMsg:
				b.broadcastTyped(MsgStarted, WireStarted{
					SessionID: ev.SessionID,
					TurnHexID: ev.TurnHexID,
					StepHexID: ev.StepHexID,
					RunHexID:  ev.RunHexID,
				})
			case events.AgentPromptMsg:
				b.broadcastTyped(MsgPrompt, WirePrompt{
					SessionID:      ev.SessionID,
					TurnHexID:      ev.TurnHexID,
					Prompt:         ev.Prompt,
					IsResume:       ev.IsResume,
					Truncated:      ev.Truncated,
					OriginalLength: ev.OriginalLength,
				})
			case events.AgentFinishedMsg:
				wf := WireFinished{SessionID: ev.SessionID, TurnHexID: ev.TurnHexID}
				wf.WaitForSpawns = ev.WaitForSpawns
				if ev.Result != nil {
					wf.ExitCode = ev.Result.ExitCode
					wf.DurationNS = int64(ev.Result.Duration)
					if ev.Result.AgentSessionID != "" {
						_ = WriteAgentSessionID(b.meta.SessionID, ev.Result.AgentSessionID)
					}
				}
				if ev.Err != nil {
					wf.Error = ev.Err.Error()
				}
				b.broadcastTyped(MsgFinished, wf)
			case events.AgentRawOutputMsg:
				spawnID := 0
				if ev.SessionID < 0 {
					spawnID = -ev.SessionID
				}
				b.broadcastTyped(MsgRaw, WireRaw{Data: ev.Data, SessionID: ev.SessionID, SpawnID: spawnID})
			case events.AgentEventMsg:
				eventJSON, err := json.Marshal(ev.Event)
				if err != nil {
					continue
				}
				b.broadcastTypedWithUpdate(MsgEvent, WireEvent{Event: eventJSON, Raw: ev.Raw, SpawnID: ev.SpawnID}, snapshotUpdate{
					model: ev.Event.Model,
				})
			case events.SpawnStatusMsg:
				spawns := make([]WireSpawnInfo, len(ev.Spawns))
				for i, sp := range ev.Spawns {
					spawns[i] = WireSpawnInfo{
						ID:            sp.ID,
						ParentTurnID:  sp.ParentTurnID,
						ParentSpawnID: sp.ParentSpawnID,
						ChildTurnID:   sp.ChildTurnID,
						Profile:       sp.Profile,
						Role:          sp.Role,
						Status:        sp.Status,
						Question:      sp.Question,
					}
				}
				b.broadcastTyped(MsgSpawn, WireSpawn{Spawns: spawns})
			case events.LoopStepStartMsg:
				if ev.RunID > 0 {
					loopRunID = ev.RunID
				}
				if ev.RunHexID != "" {
					loopRunHexID = ev.RunHexID
				}
				b.broadcastTyped(MsgLoopStepStart, WireLoopStepStart{
					RunID:      ev.RunID,
					RunHexID:   ev.RunHexID,
					StepHexID:  ev.StepHexID,
					Cycle:      ev.Cycle,
					StepIndex:  ev.StepIndex,
					Profile:    ev.Profile,
					Turns:      ev.Turns,
					TotalSteps: totalSteps,
				})
			case events.LoopStepEndMsg:
				if ev.RunID > 0 {
					loopRunID = ev.RunID
				}
				if ev.RunHexID != "" {
					loopRunHexID = ev.RunHexID
				}
				b.broadcastTyped(MsgLoopStepEnd, WireLoopStepEnd{
					RunID:      ev.RunID,
					RunHexID:   ev.RunHexID,
					StepHexID:  ev.StepHexID,
					Cycle:      ev.Cycle,
					StepIndex:  ev.StepIndex,
					Profile:    ev.Profile,
					TotalSteps: totalSteps,
				})
			}
		}
	}()

	runErr := looprun.Run(ctx, looprun.RunConfig{
		Store:           s,
		GlobalCfg:       globalCfg,
		LoopDef:         &loopDef,
		Project:         projCfg,
		AgentsCfg:       agentsCfg,
		PlanID:          cfg.PlanID,
		SessionID:       b.meta.SessionID,
		WorkDir:         workDir,
		MaxCycles:       cfg.MaxCycles,
		ResumeSessionID: cfg.ResumeSessionID,
		InitialPrompt:   cfg.InitialPrompt,
	}, eventCh)

	close(eventCh)
	<-forwardDone

	loopDone := WireLoopDone{
		RunID:    loopRunID,
		RunHexID: loopRunHexID,
		Reason:   classifyLoopDoneReason(runErr),
		Error:    donePayloadError(runErr),
	}
	b.broadcastTyped(MsgLoopDone, loopDone)

	wd := WireDone{Error: donePayloadError(runErr)}
	b.broadcastTyped(MsgDone, wd)

	return runErr
}

func applyAgentCommandOverrides(agentsCfg *agent.AgentsConfig, overrides map[string]string) {
	if agentsCfg == nil || len(overrides) == 0 {
		return
	}
	if agentsCfg.Agents == nil {
		agentsCfg.Agents = make(map[string]agent.AgentRecord)
	}
	for name, path := range overrides {
		name = strings.TrimSpace(strings.ToLower(name))
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			continue
		}
		rec := agentsCfg.Agents[name]
		rec.Name = name
		rec.Path = path
		agentsCfg.Agents[name] = rec
	}
}

func classifySessionEnd(loopErr error) (status string, errMsg string) {
	switch {
	case loopErr == nil:
		return StatusDone, ""
	case errors.Is(loopErr, context.Canceled):
		return StatusCancelled, ""
	default:
		return StatusError, loopErr.Error()
	}
}

func donePayloadError(loopErr error) string {
	if loopErr == nil || errors.Is(loopErr, context.Canceled) {
		return ""
	}
	return loopErr.Error()
}

func classifyLoopDoneReason(loopErr error) string {
	switch {
	case loopErr == nil:
		return "stopped"
	case errors.Is(loopErr, context.Canceled):
		return "cancelled"
	default:
		return "error"
	}
}

func normalizeDaemonExit(loopErr error) error {
	if errors.Is(loopErr, context.Canceled) {
		return nil
	}
	return loopErr
}
