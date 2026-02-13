package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/looprun"
	"github.com/agusx1211/adaf/internal/orchestrator"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/store"
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
	message += "; try 'adaf repair' to restore missing project metadata directories"
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
			LoopName:    cfg.Loop.Name,
			LoopSteps:   len(cfg.Loop.Steps),
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

	return normalizeDaemonExit(loopErr)
}

// broadcaster manages client connections and event broadcasting.
type broadcaster struct {
	mu               sync.Mutex
	clients          []*clientConn
	buffered         []bufferedEvent // bounded replay buffer
	eventsFile       *os.File
	meta             WireMeta
	done             bool
	nextSeq          int64
	maxReplayEvents  int
	eventsWriteError bool

	controlMu      sync.RWMutex
	controlHandler func(WireControl) WireControlResult
}

type clientConn struct {
	conn   net.Conn
	writer *bufio.Writer
	mu     sync.Mutex
}

type bufferedEvent struct {
	Seq  int64
	Line []byte
}

const defaultMaxReplayEvents = 4096

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

	// Replay a stable snapshot then register for live events.
	seq1 := b.currentSeq()
	if err := b.replayRange(cc, 1, seq1); err != nil {
		fmt.Fprintf(os.Stderr, "session %d: replay range 1..%d failed: %v\n", b.meta.SessionID, seq1, err)
	}

	seq2 := b.currentSeq()
	if err := b.replayRange(cc, seq1+1, seq2); err != nil {
		fmt.Fprintf(os.Stderr, "session %d: replay range %d..%d failed: %v\n", b.meta.SessionID, seq1+1, seq2, err)
	}

	b.mu.Lock()
	seq3 := b.nextSeq
	replay3, ok3 := b.bufferedRangeLocked(seq2+1, seq3)
	b.clients = append(b.clients, cc)
	b.mu.Unlock()

	if seq3 > seq2 {
		if ok3 {
			for _, line := range replay3 {
				cc.writeLine(line)
			}
		} else if err := b.replayFromFileRange(cc, seq2+1, seq3); err != nil {
			fmt.Fprintf(os.Stderr, "session %d: replay range %d..%d failed: %v\n", b.meta.SessionID, seq2+1, seq3, err)
		}
	}

	// Send live marker after replay/catchup.
	liveLine, _ := EncodeMsg(MsgLive, nil)
	cc.writeLine(liveLine)
	cc.flush()

	// Read control messages from the client.
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == CtrlCancel {
			cancelAgent()
			continue
		}

		msg, err := DecodeMsg([]byte(line))
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
		respLine, _ := EncodeMsg(MsgControlResult, resp)
		cc.writeLine(respLine)
		cc.flush()
	}

	// Client disconnected.
	b.removeClient(cc)
	conn.Close()
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
	defer b.mu.Unlock()
	for i, c := range b.clients {
		if c == cc {
			b.clients = append(b.clients[:i], b.clients[i+1:]...)
			return
		}
	}
}

func (b *broadcaster) currentSeq() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nextSeq
}

func (b *broadcaster) replayRange(cc *clientConn, fromSeq, toSeq int64) error {
	if toSeq < fromSeq {
		return nil
	}

	b.mu.Lock()
	lines, ok := b.bufferedRangeLocked(fromSeq, toSeq)
	b.mu.Unlock()
	if ok {
		for _, line := range lines {
			cc.writeLine(line)
		}
		return nil
	}

	return b.replayFromFileRange(cc, fromSeq, toSeq)
}

func (b *broadcaster) bufferedRangeLocked(fromSeq, toSeq int64) ([][]byte, bool) {
	if toSeq < fromSeq {
		return nil, true
	}
	if len(b.buffered) == 0 {
		return nil, false
	}

	first := b.buffered[0].Seq
	last := b.buffered[len(b.buffered)-1].Seq
	if fromSeq < first || toSeq > last {
		return nil, false
	}

	lines := make([][]byte, 0, toSeq-fromSeq+1)
	next := fromSeq
	for _, ev := range b.buffered {
		if ev.Seq < fromSeq {
			continue
		}
		if ev.Seq > toSeq {
			break
		}
		if ev.Seq != next {
			return nil, false
		}
		lines = append(lines, ev.Line)
		next++
	}
	if next != toSeq+1 {
		return nil, false
	}
	return lines, true
}

func (b *broadcaster) replayFromFileRange(cc *clientConn, fromSeq, toSeq int64) error {
	if toSeq < fromSeq {
		return nil
	}
	if b.eventsFile == nil {
		return fmt.Errorf("events file is not configured")
	}

	path := b.eventsFile.Name()
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var seq int64
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			seq++
			if seq >= fromSeq && seq <= toSeq {
				cc.writeLine(line)
			}
			if seq >= toSeq {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if seq < toSeq {
					return fmt.Errorf("events replay truncated at seq %d (wanted %d)", seq, toSeq)
				}
				return nil
			}
			return err
		}
	}
}

// broadcast sends a message to all connected clients and buffers it for replay.
func (b *broadcaster) broadcast(line []byte) {
	if b.maxReplayEvents <= 0 {
		b.maxReplayEvents = defaultMaxReplayEvents
	}
	lineCopy := append([]byte(nil), line...)

	b.mu.Lock()
	b.nextSeq++
	b.buffered = append(b.buffered, bufferedEvent{
		Seq:  b.nextSeq,
		Line: lineCopy,
	})
	if excess := len(b.buffered) - b.maxReplayEvents; excess > 0 {
		b.buffered = append([]bufferedEvent(nil), b.buffered[excess:]...)
	}

	// Write to events file.
	if _, err := b.eventsFile.Write(lineCopy); err != nil && !b.eventsWriteError {
		b.eventsWriteError = true
		fmt.Fprintf(os.Stderr, "session %d: failed to append to events file: %v\n", b.meta.SessionID, err)
	}

	clients := make([]*clientConn, len(b.clients))
	copy(clients, b.clients)
	b.mu.Unlock()

	for _, cc := range clients {
		cc.writeLine(lineCopy)
		cc.flush()
	}
}

func (b *broadcaster) markDone() {
	b.mu.Lock()
	b.done = true
	b.mu.Unlock()
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
		default:
			resp.Error = fmt.Sprintf("unsupported control action %q", req.Action)
			return resp
		}
	})
	defer b.setControlHandler(nil)

	eventCh := make(chan any, 256)
	forwardDone := make(chan struct{})
	loopRunID := 0
	loopRunHexID := ""
	go func() {
		defer close(forwardDone)
		totalSteps := len(loopDef.Steps)
		for msg := range eventCh {
			switch ev := msg.(type) {
			case runtui.AgentStartedMsg:
				line, _ := EncodeMsg(MsgStarted, WireStarted{
					SessionID: ev.SessionID,
					TurnHexID: ev.TurnHexID,
					StepHexID: ev.StepHexID,
					RunHexID:  ev.RunHexID,
				})
				b.broadcast(line)
			case runtui.AgentPromptMsg:
				line, _ := EncodeMsg(MsgPrompt, WirePrompt{
					SessionID:      ev.SessionID,
					TurnHexID:      ev.TurnHexID,
					Prompt:         ev.Prompt,
					IsResume:       ev.IsResume,
					Truncated:      ev.Truncated,
					OriginalLength: ev.OriginalLength,
				})
				b.broadcast(line)
			case runtui.AgentFinishedMsg:
				wf := WireFinished{SessionID: ev.SessionID, TurnHexID: ev.TurnHexID}
				wf.WaitForSpawns = ev.WaitForSpawns
				if ev.Result != nil {
					wf.ExitCode = ev.Result.ExitCode
					wf.DurationNS = ev.Result.Duration
				}
				if ev.Err != nil {
					wf.Error = ev.Err.Error()
				}
				line, _ := EncodeMsg(MsgFinished, wf)
				b.broadcast(line)
			case runtui.AgentRawOutputMsg:
				line, _ := EncodeMsg(MsgRaw, WireRaw{Data: ev.Data, SessionID: ev.SessionID})
				b.broadcast(line)
			case runtui.AgentEventMsg:
				eventJSON, err := json.Marshal(ev.Event)
				if err != nil {
					continue
				}
				line, _ := EncodeMsg(MsgEvent, WireEvent{Event: eventJSON, Raw: ev.Raw})
				b.broadcast(line)
			case runtui.SpawnStatusMsg:
				spawns := make([]WireSpawnInfo, len(ev.Spawns))
				for i, sp := range ev.Spawns {
					spawns[i] = WireSpawnInfo{
						ID:           sp.ID,
						ParentTurnID: sp.ParentTurnID,
						Profile:      sp.Profile,
						Status:       sp.Status,
						Question:     sp.Question,
					}
				}
				line, _ := EncodeMsg(MsgSpawn, WireSpawn{Spawns: spawns})
				b.broadcast(line)
			case runtui.LoopStepStartMsg:
				if ev.RunID > 0 {
					loopRunID = ev.RunID
				}
				if ev.RunHexID != "" {
					loopRunHexID = ev.RunHexID
				}
				line, _ := EncodeMsg(MsgLoopStepStart, WireLoopStepStart{
					RunID:      ev.RunID,
					RunHexID:   ev.RunHexID,
					StepHexID:  ev.StepHexID,
					Cycle:      ev.Cycle,
					StepIndex:  ev.StepIndex,
					Profile:    ev.Profile,
					Turns:      ev.Turns,
					TotalSteps: totalSteps,
				})
				b.broadcast(line)
			case runtui.LoopStepEndMsg:
				if ev.RunID > 0 {
					loopRunID = ev.RunID
				}
				if ev.RunHexID != "" {
					loopRunHexID = ev.RunHexID
				}
				line, _ := EncodeMsg(MsgLoopStepEnd, WireLoopStepEnd{
					RunID:      ev.RunID,
					RunHexID:   ev.RunHexID,
					StepHexID:  ev.StepHexID,
					Cycle:      ev.Cycle,
					StepIndex:  ev.StepIndex,
					Profile:    ev.Profile,
					TotalSteps: totalSteps,
				})
				b.broadcast(line)
			}
		}
	}()

	runErr := looprun.Run(ctx, looprun.RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   &loopDef,
		Project:   projCfg,
		AgentsCfg: agentsCfg,
		PlanID:    cfg.PlanID,
		SessionID: b.meta.SessionID,
		WorkDir:   workDir,
		MaxCycles: cfg.MaxCycles,
	}, eventCh)

	close(eventCh)
	<-forwardDone

	loopDone := WireLoopDone{
		RunID:    loopRunID,
		RunHexID: loopRunHexID,
		Reason:   classifyLoopDoneReason(runErr),
		Error:    donePayloadError(runErr),
	}
	line, _ := EncodeMsg(MsgLoopDone, loopDone)
	b.broadcast(line)

	wd := WireDone{Error: donePayloadError(runErr)}
	line, _ = EncodeMsg(MsgDone, wd)
	b.broadcast(line)
	b.markDone()

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
