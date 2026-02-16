package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestClassifySessionEnd(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
		wantMsg    string
	}{
		{
			name:       "done",
			err:        nil,
			wantStatus: "done",
			wantMsg:    "",
		},
		{
			name:       "cancelled direct",
			err:        context.Canceled,
			wantStatus: "cancelled",
			wantMsg:    "",
		},
		{
			name:       "cancelled wrapped",
			err:        fmt.Errorf("agent run failed: %w", context.Canceled),
			wantStatus: "cancelled",
			wantMsg:    "",
		},
		{
			name:       "error",
			err:        errors.New("boom"),
			wantStatus: "error",
			wantMsg:    "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMsg := classifySessionEnd(tt.err)
			if gotStatus != tt.wantStatus {
				t.Fatalf("status = %q, want %q", gotStatus, tt.wantStatus)
			}
			if gotMsg != tt.wantMsg {
				t.Fatalf("err msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestDonePayloadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "cancelled direct", err: context.Canceled, want: ""},
		{name: "cancelled wrapped", err: fmt.Errorf("wrap: %w", context.Canceled), want: ""},
		{name: "error", err: errors.New("failed"), want: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := donePayloadError(tt.err)
			if got != tt.want {
				t.Fatalf("donePayloadError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeDaemonExit(t *testing.T) {
	if err := normalizeDaemonExit(context.Canceled); err != nil {
		t.Fatalf("normalizeDaemonExit(context.Canceled) = %v, want nil", err)
	}

	wrapped := fmt.Errorf("wrapped: %w", context.Canceled)
	if err := normalizeDaemonExit(wrapped); err != nil {
		t.Fatalf("normalizeDaemonExit(wrapped canceled) = %v, want nil", err)
	}

	expected := errors.New("boom")
	if err := normalizeDaemonExit(expected); !errors.Is(err, expected) {
		t.Fatalf("normalizeDaemonExit(non-cancelled) = %v, want %v", err, expected)
	}
}

func TestBuildDaemonStartupErrorIncludesLogSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(SessionDir(42), 0755); err != nil {
		t.Fatalf("MkdirAll(SessionDir): %v", err)
	}
	logPath := DaemonLogPath(42)
	logLine := "\x1b[31mError: step 0 failed: open .adaf/logs/1.json: no such file or directory\x1b[0m\n"
	if err := os.WriteFile(logPath, []byte(logLine), 0644); err != nil {
		t.Fatalf("WriteFile(logPath): %v", err)
	}

	err := buildDaemonStartupError(42, "daemon did not create socket within 10 seconds", nil)
	if err == nil {
		t.Fatal("buildDaemonStartupError returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "step 0 failed: open .adaf/logs/1.json: no such file or directory") {
		t.Fatalf("error missing log summary: %q", msg)
	}
	if strings.Contains(msg, "\x1b[31m") {
		t.Fatalf("error contains ANSI escapes: %q", msg)
	}
	if !strings.Contains(msg, "adaf init") {
		t.Fatalf("error missing init hint: %q", msg)
	}
	if !strings.Contains(msg, logPath) {
		t.Fatalf("error missing daemon log path: %q", msg)
	}
}

func TestBuildDaemonStartupErrorFallsBackToWaitErr(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(SessionDir(7), 0755); err != nil {
		t.Fatalf("MkdirAll(SessionDir): %v", err)
	}

	waitErr := errors.New("exit status 1")
	err := buildDaemonStartupError(7, "daemon exited before creating socket", waitErr)
	if err == nil {
		t.Fatal("buildDaemonStartupError returned nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exit status 1") {
		t.Fatalf("error missing waitErr details: %q", msg)
	}
	if !strings.Contains(msg, "session #7") {
		t.Fatalf("error missing session id: %q", msg)
	}
}

func TestClassifyLoopDoneReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: "stopped"},
		{name: "cancelled direct", err: context.Canceled, want: "cancelled"},
		{name: "cancelled wrapped", err: fmt.Errorf("wrapped: %w", context.Canceled), want: "cancelled"},
		{name: "error", err: errors.New("boom"), want: "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyLoopDoneReason(tt.err); got != tt.want {
				t.Fatalf("classifyLoopDoneReason(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestRunLoopBroadcastsLoopLifecycleMessages(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 1},
	}

	cfg := &DaemonConfig{
		ProjectDir:  projectDir,
		ProjectName: "proj",
		WorkDir:     projectDir,
		PlanID:      "",
		ProfileName: "p1",
		AgentName:   "generic",
		Loop: config.LoopDef{
			Name: "loop-test",
			Steps: []config.LoopStep{
				{
					Profile:      "p1",
					Turns:        1,
					Instructions: "Say hello",
				},
			},
		},
		Profiles: []config.Profile{
			{
				Name:  "p1",
				Agent: "generic",
			},
		},
		MaxCycles: 1,
		AgentCommandOverrides: map[string]string{
			"generic": "/bin/cat",
		},
	}

	if err := b.runLoop(context.Background(), cfg); err != nil {
		t.Fatalf("runLoop: %v", err)
	}

	var hasStepStart bool
	var hasStepEnd bool
	var hasLoopDone bool
	var hasDone bool
	var loopDone WireLoopDone

	eventsFile.Close()
	eventsData, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(eventsData)), "\n") {
		if line == "" {
			continue
		}
		msg, err := DecodeMsg([]byte(line))
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		switch msg.Type {
		case MsgLoopStepStart:
			hasStepStart = true
		case MsgLoopStepEnd:
			hasStepEnd = true
		case MsgLoopDone:
			hasLoopDone = true
			data, err := DecodeData[WireLoopDone](msg)
			if err != nil {
				t.Fatalf("DecodeData[WireLoopDone]: %v", err)
			}
			loopDone = *data
		case MsgDone:
			hasDone = true
		}
	}

	if !hasStepStart {
		t.Fatal("missing MsgLoopStepStart in broadcast stream")
	}
	if !hasStepEnd {
		t.Fatal("missing MsgLoopStepEnd in broadcast stream")
	}
	if !hasLoopDone {
		t.Fatal("missing MsgLoopDone in broadcast stream")
	}
	if !hasDone {
		t.Fatal("missing MsgDone in broadcast stream")
	}
	if loopDone.Reason != "stopped" {
		t.Fatalf("loop done reason = %q, want %q", loopDone.Reason, "stopped")
	}
	if loopDone.Error != "" {
		t.Fatalf("loop done error = %q, want empty", loopDone.Error)
	}
	if loopDone.RunID == 0 {
		t.Fatal("loop done run_id = 0, want non-zero")
	}
}

// wsTestServer starts a broadcaster-backed HTTP server and returns a WebSocket
// connection to it. The connection reads messages as JSON text frames.
func wsTestServer(t *testing.T, b *broadcaster) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.handleWSClient(w, r, func() {})
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	ws, _, err := websocket.Dial(ctx, "ws://"+srv.Listener.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	ws.SetReadLimit(4 * 1024 * 1024)
	t.Cleanup(func() { ws.CloseNow() })
	return ws
}

// readWSMsg reads one WebSocket text frame and decodes it as a WireMsg.
func readWSMsg(t *testing.T, ws *websocket.Conn) *WireMsg {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatalf("ws.Read: %v", err)
	}
	msg, err := DecodeMsg(data)
	if err != nil {
		t.Fatalf("DecodeMsg: %v", err)
	}
	return msg
}

// readWSMsgsUntil reads messages until it finds one with the given type or times out.
func readWSMsgsUntil(t *testing.T, ws *websocket.Conn, targetType string) (*WireMsg, []string) {
	t.Helper()
	var order []string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			t.Fatalf("ws.Read waiting for %q: %v (seen: %v)", targetType, err, order)
		}
		msg, err := DecodeMsg(data)
		if err != nil {
			continue
		}
		order = append(order, msg.Type)
		if msg.Type == targetType {
			return msg, order
		}
	}
}

func TestHandleClientSendsSnapshotRecentOutput(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 7},
	}

	for i := 0; i < 5; i++ {
		b.broadcastTyped(MsgRaw, WireRaw{Data: fmt.Sprintf("line-%d", i+1)})
	}

	ws := wsTestServer(t, b)

	// Read meta.
	metaMsg := readWSMsg(t, ws)
	if metaMsg.Type != MsgMeta {
		t.Fatalf("expected meta, got %q", metaMsg.Type)
	}

	// Read snapshot.
	snapMsg := readWSMsg(t, ws)
	if snapMsg.Type != MsgSnapshot {
		t.Fatalf("expected snapshot, got %q", snapMsg.Type)
	}
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	recentRawCount := 0
	for _, recent := range snapshot.Recent {
		if recent.Type == MsgRaw {
			recentRawCount++
		}
	}
	if recentRawCount != 5 {
		t.Fatalf("snapshot recent raw count = %d, want 5", recentRawCount)
	}

	// Read live marker.
	liveMsg := readWSMsg(t, ws)
	if liveMsg.Type != MsgLive {
		t.Fatalf("expected live, got %q", liveMsg.Type)
	}
}

func TestHandleClientSnapshotCarriesCurrentState(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta: WireMeta{
			SessionID: 12,
			AgentName: "codex",
		},
	}

	b.broadcastTyped(MsgLoopStepStart, WireLoopStepStart{
		RunID:      3,
		RunHexID:   "runhex",
		StepHexID:  "stephex",
		Cycle:      1,
		StepIndex:  0,
		Profile:    "reviewer",
		TotalSteps: 4,
	})
	b.broadcastTyped(MsgStarted, WireStarted{
		SessionID: 42,
		TurnHexID: "turnhex",
	})

	ws := wsTestServer(t, b)

	// Read meta.
	readWSMsg(t, ws)

	// Read snapshot.
	snapMsg := readWSMsg(t, ws)
	if snapMsg.Type != MsgSnapshot {
		t.Fatalf("expected snapshot, got %q", snapMsg.Type)
	}
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}

	if snapshot.Loop.RunID != 3 || snapshot.Loop.StepIndex != 0 || snapshot.Loop.Profile != "reviewer" {
		t.Fatalf("unexpected snapshot loop: %+v", snapshot.Loop)
	}
	if snapshot.Session == nil {
		t.Fatal("snapshot missing session state")
	}
	if snapshot.Session.SessionID != 42 || snapshot.Session.TurnHexID != "turnhex" {
		t.Fatalf("unexpected snapshot session: %+v", snapshot.Session)
	}
	if snapshot.Session.Agent != "codex" || snapshot.Session.Status != "running" {
		t.Fatalf("unexpected snapshot session identity/status: %+v", snapshot.Session)
	}
}

func TestSnapshotSessionActionTracksMsgEventAsResponding(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta: WireMeta{
			SessionID: 121,
			AgentName: "codex",
		},
	}
	b.broadcastTyped(MsgStarted, WireStarted{
		SessionID: 42,
		TurnHexID: "turnhex",
	})
	b.broadcastTyped(MsgEvent, WireEvent{
		Event: json.RawMessage(`{"type":"assistant","model":"gpt-5"}`),
	})

	ws := wsTestServer(t, b)
	readWSMsg(t, ws) // meta

	snapMsg := readWSMsg(t, ws)
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if snapshot == nil || snapshot.Session == nil {
		t.Fatal("missing snapshot session")
	}
	if snapshot.Session.Action != "responding" {
		t.Fatalf("snapshot session action = %q, want %q", snapshot.Session.Action, "responding")
	}
	if snapshot.Session.Model != "gpt-5" {
		t.Fatalf("snapshot session model = %q, want %q", snapshot.Session.Model, "gpt-5")
	}
}

func TestLoopStepStartKeepsExistingTotalStepsWhenUnset(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 120},
		snapshot: WireSnapshot{
			Loop: WireSnapshotLoop{
				TotalSteps: 6,
			},
		},
	}

	b.broadcastTyped(MsgLoopStepStart, WireLoopStepStart{
		RunID:     9,
		RunHexID:  "runhex",
		StepHexID: "stephex",
		Cycle:     0,
		StepIndex: 1,
		Profile:   "reviewer",
	})

	ws := wsTestServer(t, b)
	readWSMsg(t, ws) // meta

	snapMsg := readWSMsg(t, ws)
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if snapshot == nil {
		t.Fatal("missing MsgSnapshot")
	}
	if snapshot.Loop.TotalSteps != 6 {
		t.Fatalf("snapshot loop total_steps = %d, want 6", snapshot.Loop.TotalSteps)
	}
}

func TestHandleClientQueuedLiveEventsAfterSnapshotAndLiveMarker(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 81},
	}
	b.broadcastTyped(MsgRaw, WireRaw{Data: "before", SessionID: 1})

	// Start a test server — this triggers WebSocket accept + handleWSClient.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.handleWSClient(w, r, func() {})
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	ws, _, err := websocket.Dial(ctx, "ws://"+srv.Listener.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	ws.SetReadLimit(4 * 1024 * 1024)
	t.Cleanup(func() { ws.CloseNow() })

	// Read meta.
	metaMsg := readWSMsg(t, ws)
	if metaMsg.Type != MsgMeta {
		t.Fatalf("expected meta, got %q", metaMsg.Type)
	}

	// Wait for client to be registered.
	deadline := time.Now().Add(2 * time.Second)
	registered := false
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.clients)
		b.mu.Unlock()
		if n == 1 {
			registered = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !registered {
		t.Fatal("client did not register in time")
	}

	// Broadcast a live event after registration.
	b.broadcastTyped(MsgRaw, WireRaw{Data: "after", SessionID: 1})

	// Read remaining messages.
	var snapshot *WireSnapshot
	liveSeen := false
	rawAfterLive := 0
	for {
		msg := readWSMsg(t, ws)
		switch msg.Type {
		case MsgSnapshot:
			data, err := DecodeData[WireSnapshot](msg)
			if err != nil {
				t.Fatalf("DecodeData[WireSnapshot]: %v", err)
			}
			snapshot = data
		case MsgLive:
			liveSeen = true
		case MsgRaw:
			if !liveSeen {
				t.Fatal("received MsgRaw before MsgLive")
			}
			data, err := DecodeData[WireRaw](msg)
			if err != nil {
				t.Fatalf("DecodeData[WireRaw]: %v", err)
			}
			if data.Data == "after" {
				rawAfterLive++
				goto done
			}
		}
	}

done:
	if snapshot == nil {
		t.Fatal("missing MsgSnapshot")
	}
	if !liveSeen {
		t.Fatal("missing MsgLive")
	}
	snapshotRawCount := 0
	for _, recent := range snapshot.Recent {
		if recent.Type != MsgRaw {
			continue
		}
		data, err := DecodeData[WireRaw](&recent)
		if err != nil {
			t.Fatalf("DecodeData[WireRaw](recent): %v", err)
		}
		if data.Data == "before" {
			snapshotRawCount++
		}
		if data.Data == "after" {
			t.Fatal("post-registration raw event leaked into snapshot recent")
		}
	}
	if snapshotRawCount != 1 {
		t.Fatalf("snapshot raw count = %d, want 1", snapshotRawCount)
	}
	if rawAfterLive != 1 {
		t.Fatalf("post-live raw count = %d, want 1", rawAfterLive)
	}
}

func TestSnapshotRecentClearsOnLoopStepStart(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 84},
	}
	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 1, Data: "stale"})
	b.broadcastTyped(MsgLoopStepStart, WireLoopStepStart{
		RunID:      3,
		RunHexID:   "run-hex",
		StepHexID:  "step-hex",
		Cycle:      0,
		StepIndex:  1,
		Profile:    "reviewer",
		TotalSteps: 4,
	})
	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 1, Data: "fresh"})

	ws := wsTestServer(t, b)
	readWSMsg(t, ws) // meta

	snapMsg := readWSMsg(t, ws)
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if snapshot == nil {
		t.Fatal("missing MsgSnapshot")
	}
	var staleSeen, freshSeen int
	for _, recent := range snapshot.Recent {
		if recent.Type != MsgRaw {
			continue
		}
		data, err := DecodeData[WireRaw](&recent)
		if err != nil {
			t.Fatalf("DecodeData[WireRaw](recent): %v", err)
		}
		switch data.Data {
		case "stale":
			staleSeen++
		case "fresh":
			freshSeen++
		}
	}
	if staleSeen != 0 {
		t.Fatalf("stale recent entries = %d, want 0", staleSeen)
	}
	if freshSeen != 1 {
		t.Fatalf("fresh recent entries = %d, want 1", freshSeen)
	}
}

func TestSnapshotRecentPreservesPromptAfterStarted(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 86, AgentName: "claude"},
	}
	b.broadcastTyped(MsgPrompt, WirePrompt{
		SessionID: 5,
		TurnHexID: "hex5",
		Prompt:    "do the thing",
	})
	b.broadcastTyped(MsgStarted, WireStarted{
		SessionID: 5,
		TurnHexID: "hex5",
	})
	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 5, Data: "output"})

	ws := wsTestServer(t, b)
	readWSMsg(t, ws) // meta

	snapMsg := readWSMsg(t, ws)
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if snapshot == nil {
		t.Fatal("missing MsgSnapshot")
	}
	var promptCount int
	for _, recent := range snapshot.Recent {
		if recent.Type == MsgPrompt {
			promptCount++
			data, err := DecodeData[WirePrompt](&recent)
			if err != nil {
				t.Fatalf("DecodeData[WirePrompt]: %v", err)
			}
			if data.Prompt != "do the thing" {
				t.Fatalf("prompt = %q, want %q", data.Prompt, "do the thing")
			}
		}
	}
	if promptCount != 1 {
		t.Fatalf("prompt entries in snapshot recent = %d, want 1", promptCount)
	}
}

func TestBroadcastSkipsClientBeforeSnapshotBoundary(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 85},
	}

	ws := wsTestServer(t, b)
	_, order := readWSMsgsUntil(t, ws, MsgLive)
	if !containsType(order, MsgLive) {
		t.Fatal("missing MsgLive")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.clients)
		b.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	b.mu.Lock()
	if len(b.clients) != 1 {
		b.mu.Unlock()
		t.Fatalf("registered clients = %d, want 1", len(b.clients))
	}
	b.clients[0].minSeq = b.streamSeq + 2
	b.mu.Unlock()

	type readResult struct {
		msg *WireMsg
		err error
	}
	readCh := make(chan readResult, 1)
	go func() {
		readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer readCancel()
		_, data, err := ws.Read(readCtx)
		if err != nil {
			readCh <- readResult{err: err}
			return
		}
		msg, err := DecodeMsg(data)
		if err != nil {
			readCh <- readResult{err: err}
			return
		}
		readCh <- readResult{msg: msg}
	}()

	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 1, Data: "seq-skip"})
	select {
	case got := <-readCh:
		if got.err != nil {
			t.Fatalf("unexpected read error below snapshot boundary: %v", got.err)
		}
		t.Fatalf("unexpected message below snapshot boundary: %q", got.msg.Type)
	case <-time.After(200 * time.Millisecond):
	}

	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 1, Data: "seq-hit"})
	var rawMsg *WireMsg
	select {
	case got := <-readCh:
		if got.err != nil {
			t.Fatalf("read after boundary broadcast: %v", got.err)
		}
		rawMsg = got.msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for boundary message")
	}
	if rawMsg.Type != MsgRaw {
		t.Fatalf("expected raw message at snapshot boundary, got %q", rawMsg.Type)
	}
	data, err := DecodeData[WireRaw](rawMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireRaw]: %v", err)
	}
	if data.Data != "seq-hit" {
		t.Fatalf("raw data = %q, want %q", data.Data, "seq-hit")
	}
}

func TestCloseAllClientsClosesGoingAway(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 88},
	}

	ws := wsTestServer(t, b)
	_, order := readWSMsgsUntil(t, ws, MsgLive)
	if !containsType(order, MsgLive) {
		t.Fatal("missing MsgLive")
	}

	b.closeAllClients(websocket.StatusGoingAway, "daemon shutting down")

	b.mu.Lock()
	n := len(b.clients)
	b.mu.Unlock()
	if n != 0 {
		t.Fatalf("client count after closeAllClients = %d, want 0", n)
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer readCancel()
	_, _, err = ws.Read(readCtx)
	if err == nil {
		t.Fatal("expected websocket close after closeAllClients")
	}
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway && got != websocket.StatusCode(-1) {
		t.Fatalf("close status = %v, want %v or %v", got, websocket.StatusGoingAway, websocket.StatusCode(-1))
	}
}

func TestBroadcastRemovesClientAfterWriteFailure(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 10},
	}

	ws := wsTestServer(t, b)
	_, order := readWSMsgsUntil(t, ws, MsgLive)
	if !containsType(order, MsgLive) {
		t.Fatal("missing MsgLive")
	}

	var cc *clientConn
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		if len(b.clients) == 1 {
			cc = b.clients[0]
		}
		b.mu.Unlock()
		if cc != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cc == nil {
		t.Fatal("client did not register in time")
	}

	cc.close(websocket.StatusNormalClosure, "test close")
	b.broadcastTyped(MsgRaw, WireRaw{Data: "after-close"})

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.clients)
		b.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("client was not removed after write failure")
}

func TestHandleClientFinishedDaemonSendsSnapshotAndTerminalMessages(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 82, AgentName: "codex"},
		lastModel:  "gpt-5",
	}
	b.broadcastTyped(MsgLoopStepStart, WireLoopStepStart{
		RunID:      11,
		RunHexID:   "runhex",
		StepHexID:  "stephex",
		Cycle:      1,
		StepIndex:  2,
		Profile:    "reviewer",
		TotalSteps: 4,
	})
	b.broadcastTyped(MsgStarted, WireStarted{
		SessionID: 42,
		TurnHexID: "turnhex",
	})
	b.broadcastTyped(MsgRaw, WireRaw{
		SessionID: 42,
		Data:      "final output",
	})
	b.broadcastTyped(MsgLoopDone, WireLoopDone{
		RunID:    11,
		RunHexID: "runhex",
		Reason:   "stopped",
	})
	b.broadcastTyped(MsgDone, WireDone{})

	ws := wsTestServer(t, b)

	// Collect all messages.
	var (
		snapshot *WireSnapshot
		order    []string
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			break
		}
		msg, err := DecodeMsg(data)
		if err != nil {
			continue
		}
		order = append(order, msg.Type)
		switch msg.Type {
		case MsgSnapshot:
			d, _ := DecodeData[WireSnapshot](msg)
			snapshot = d
		case MsgDone:
			goto doneFinished
		}
	}

doneFinished:
	if snapshot == nil {
		t.Fatal("missing MsgSnapshot")
	}
	if snapshot.Loop.RunID != 11 || snapshot.Loop.StepIndex != 2 {
		t.Fatalf("unexpected snapshot loop: %+v", snapshot.Loop)
	}
	if snapshot.Session == nil {
		t.Fatal("snapshot missing session")
	}
	if snapshot.Session.SessionID != 42 || snapshot.Session.Model != "gpt-5" {
		t.Fatalf("unexpected snapshot session: %+v", snapshot.Session)
	}
	if containsType(order, MsgLive) {
		t.Fatalf("finished daemon should not emit MsgLive: %v", order)
	}
	if !containsType(order, MsgLoopDone) || !containsType(order, MsgDone) {
		t.Fatalf("missing expected message types in order: %v", order)
	}
	if typeIndex(order, MsgLoopDone) > typeIndex(order, MsgDone) {
		t.Fatalf("MsgLoopDone appears after MsgDone: %v", order)
	}
}

func TestEncodeBoundedSnapshotTrimsLargeRecentPayload(t *testing.T) {
	huge := strings.Repeat("x", 2*1024*1024)
	rawLine, err := EncodeMsg(MsgRaw, WireRaw{SessionID: 1, Data: huge})
	if err != nil {
		t.Fatalf("EncodeMsg(raw): %v", err)
	}
	rawMsg, err := DecodeMsg(rawLine)
	if err != nil {
		t.Fatalf("DecodeMsg(raw): %v", err)
	}

	line, err := encodeBoundedSnapshot(WireSnapshot{
		Loop:   WireSnapshotLoop{RunID: 1, Cycle: 0, StepIndex: 0},
		Recent: []WireMsg{*rawMsg},
	})
	if err != nil {
		t.Fatalf("encodeBoundedSnapshot: %v", err)
	}
	if len(line) > snapshotWireByteLimit {
		t.Fatalf("snapshot line size = %d, want <= %d", len(line), snapshotWireByteLimit)
	}

	wire, err := DecodeMsg(line)
	if err != nil {
		t.Fatalf("DecodeMsg(snapshot): %v", err)
	}
	snapshot, err := DecodeData[WireSnapshot](wire)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if len(snapshot.Recent) != 0 {
		t.Fatalf("expected recent to be trimmed, got %d entries", len(snapshot.Recent))
	}
}

func TestEncodeBoundedSnapshotDropsOversizedSpawns(t *testing.T) {
	hugeQuestion := strings.Repeat("q", 2*1024*1024)
	line, err := encodeBoundedSnapshot(WireSnapshot{
		Loop: WireSnapshotLoop{RunID: 1, Cycle: 0, StepIndex: 0},
		Spawns: []WireSpawnInfo{
			{ID: 1, ParentTurnID: 1, Profile: "devstral2", Status: "running", Question: hugeQuestion},
		},
		Session: &WireSnapshotSession{
			SessionID: 1,
			Agent:     "codex",
			Status:    "running",
			Action:    "responding",
		},
	})
	if err != nil {
		t.Fatalf("encodeBoundedSnapshot: %v", err)
	}
	if len(line) > snapshotWireByteLimit {
		t.Fatalf("snapshot line size = %d, want <= %d", len(line), snapshotWireByteLimit)
	}

	wire, err := DecodeMsg(line)
	if err != nil {
		t.Fatalf("DecodeMsg(snapshot): %v", err)
	}
	snapshot, err := DecodeData[WireSnapshot](wire)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if len(snapshot.Spawns) != 0 {
		t.Fatalf("expected spawns to be trimmed, got %d entries", len(snapshot.Spawns))
	}
	if snapshot.Session == nil || snapshot.Session.SessionID != 1 {
		t.Fatalf("expected session to survive trimming, got %+v", snapshot.Session)
	}
}

func TestDecodeWireDataDoesNotUsePointerPayloadFallback(t *testing.T) {
	wire, err := EncodeMsg(MsgStarted, WireStarted{SessionID: 7})
	if err != nil {
		t.Fatalf("EncodeMsg(started): %v", err)
	}
	msg, err := DecodeMsg(wire)
	if err != nil {
		t.Fatalf("DecodeMsg(started): %v", err)
	}

	got, ok := decodeWireData[WireStarted](msg, &WireStarted{SessionID: 99})
	if !ok {
		t.Fatal("decodeWireData returned ok=false for valid wire payload")
	}
	if got.SessionID != 7 {
		t.Fatalf("decoded session_id = %d, want %d", got.SessionID, 7)
	}

	_, ok = decodeWireData[WireStarted](&WireMsg{Type: MsgStarted}, &WireStarted{SessionID: 99})
	if ok {
		t.Fatal("decodeWireData accepted pointer payload without wire data")
	}
}

func TestTruncateSnapshotFieldPreservesUTF8(t *testing.T) {
	in := "ab🙂cd"
	got := truncateSnapshotField(in, 3)
	if got != "ab🙂" {
		t.Fatalf("truncateSnapshotField(%q, 3) = %q, want %q", in, got, "ab🙂")
	}
}

func TestMsgFinishedCreatesSnapshotSessionWithLastModel(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 83, AgentName: "codex"},
		lastModel:  "gpt-5",
	}
	b.broadcastTyped(MsgFinished, WireFinished{
		SessionID: 7,
		TurnHexID: "turnhex",
		ExitCode:  0,
	})

	ws := wsTestServer(t, b)
	readWSMsg(t, ws) // meta

	snapMsg := readWSMsg(t, ws)
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if snapshot == nil || snapshot.Session == nil {
		t.Fatal("missing snapshot session")
	}
	if snapshot.Session.Model != "gpt-5" {
		t.Fatalf("snapshot session model = %q, want %q", snapshot.Session.Model, "gpt-5")
	}
}

func TestMsgFinishedErrorTakesPrecedenceOverWaitForSpawns(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 86, AgentName: "codex"},
	}
	b.broadcastTyped(MsgFinished, WireFinished{
		SessionID:     7,
		TurnHexID:     "turnhex",
		WaitForSpawns: true,
		ExitCode:      1,
		Error:         "boom",
	})

	ws := wsTestServer(t, b)
	readWSMsg(t, ws) // meta

	snapMsg := readWSMsg(t, ws)
	snapshot, err := DecodeData[WireSnapshot](snapMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireSnapshot]: %v", err)
	}
	if snapshot == nil || snapshot.Session == nil {
		t.Fatal("missing snapshot session")
	}
	if snapshot.Session.Status != "failed" || snapshot.Session.Action != "error" {
		t.Fatalf("unexpected finished error state: status=%q action=%q", snapshot.Session.Status, snapshot.Session.Action)
	}
}

func TestHandleClientDoesNotDuplicateDoneAfterDoneBroadcast(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 8},
	}

	b.broadcastTyped(MsgDone, WireDone{})

	ws := wsTestServer(t, b)

	// Collect all messages until the server closes the connection.
	var order []string
	doneCount := 0
	liveSeen := false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			break
		}
		msg, err := DecodeMsg(data)
		if err != nil {
			continue
		}
		order = append(order, msg.Type)
		switch msg.Type {
		case MsgDone:
			doneCount++
		case MsgLive:
			liveSeen = true
		}
	}

	if liveSeen {
		t.Fatalf("unexpected MsgLive for finished daemon: %v", order)
	}
	if doneCount != 1 {
		t.Fatalf("done message count = %d, want 1", doneCount)
	}
	if typeIndex(order, MsgSnapshot) == -1 || typeIndex(order, MsgDone) == -1 {
		t.Fatalf("missing MsgSnapshot/MsgDone in order: %v", order)
	}
	if typeIndex(order, MsgSnapshot) > typeIndex(order, MsgDone) {
		t.Fatalf("MsgDone arrived before MsgSnapshot: %v", order)
	}
}

func TestBroadcasterConcurrentAttachAndBroadcast(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 9},
	}

	const clients = 5
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.handleWSClient(w, r, func() {})
	}))
	t.Cleanup(srv.Close)

	clientDone := make(chan error, clients)
	for i := 0; i < clients; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ws, _, err := websocket.Dial(ctx, "ws://"+srv.Listener.Addr().String()+"/", nil)
			if err != nil {
				clientDone <- err
				return
			}
			ws.SetReadLimit(4 * 1024 * 1024)
			defer ws.CloseNow()

			for {
				_, data, err := ws.Read(ctx)
				if err != nil {
					clientDone <- err
					return
				}
				msg, err := DecodeMsg(data)
				if err != nil {
					clientDone <- err
					return
				}
				if msg.Type == MsgLive {
					clientDone <- nil
					return
				}
			}
		}()
	}

	for i := 0; i < 64; i++ {
		b.broadcastTyped(MsgRaw, WireRaw{Data: fmt.Sprintf("burst-%d", i)})
	}

	for i := 0; i < clients; i++ {
		select {
		case err := <-clientDone:
			if err != nil {
				t.Fatalf("client %d error: %v", i, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("client %d timed out waiting for MsgLive", i)
		}
	}
}

func TestHandleClientControlRequest(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 11},
	}
	b.setControlHandler(func(req WireControl) WireControlResult {
		if req.Action != "spawn" {
			return WireControlResult{Action: req.Action, OK: false, Error: "unexpected action"}
		}
		return WireControlResult{
			Action:  "spawn",
			OK:      true,
			SpawnID: 42,
		}
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.handleWSClient(w, r, func() {})
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	ws, _, err := websocket.Dial(ctx, "ws://"+srv.Listener.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	ws.SetReadLimit(4 * 1024 * 1024)
	t.Cleanup(func() { ws.CloseNow() })

	// Wait for live marker.
	_, order := readWSMsgsUntil(t, ws, MsgLive)
	if !containsType(order, MsgLive) {
		t.Fatal("missing MsgLive")
	}

	// Send control request.
	line, err := EncodeMsg(MsgControl, WireControl{
		Action: "spawn",
		Spawn: &WireControlSpawn{
			ParentTurnID:  1,
			ParentProfile: "manager",
			ChildProfile:  "devstral2",
			Task:          "check files",
		},
	})
	if err != nil {
		t.Fatalf("EncodeMsg(control): %v", err)
	}
	// Strip trailing newline for WebSocket frame.
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	writeCtx, writeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer writeCancel()
	if err := ws.Write(writeCtx, websocket.MessageText, line); err != nil {
		t.Fatalf("writing control request: %v", err)
	}

	// Read until we get control result.
	controlMsg, _ := readWSMsgsUntil(t, ws, MsgControlResult)
	got, err := DecodeData[WireControlResult](controlMsg)
	if err != nil {
		t.Fatalf("DecodeData[WireControlResult]: %v", err)
	}
	if !got.OK {
		t.Fatalf("control response ok=false, error=%q", got.Error)
	}
	if got.SpawnID != 42 {
		t.Fatalf("spawn_id = %d, want 42", got.SpawnID)
	}
}

func containsType(types []string, want string) bool {
	return typeIndex(types, want) >= 0
}

func typeIndex(types []string, want string) int {
	for i, v := range types {
		if v == want {
			return i
		}
	}
	return -1
}
