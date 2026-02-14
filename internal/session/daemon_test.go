package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(msg, "adaf repair") {
		t.Fatalf("error missing repair hint: %q", msg)
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

	f, err := os.Open(eventsPath)
	if err != nil {
		t.Fatalf("open events file for read: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
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
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
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
		line, err := EncodeMsg(MsgRaw, WireRaw{Data: fmt.Sprintf("line-%d", i+1)})
		if err != nil {
			t.Fatalf("EncodeMsg raw: %v", err)
		}
		b.broadcast(line)
	}

	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(peerConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	snapshotSeen := false
	recentRawCount := 0
	liveSeen := false
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		switch msg.Type {
		case MsgSnapshot:
			data, err := DecodeData[WireSnapshot](msg)
			if err != nil {
				t.Fatalf("DecodeData[WireSnapshot]: %v", err)
			}
			snapshotSeen = true
			for _, recent := range data.Recent {
				if recent.Type == MsgRaw {
					recentRawCount++
				}
			}
		case MsgLive:
			liveSeen = true
			goto done
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

done:
	_ = peerConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
	}

	if !liveSeen {
		t.Fatal("missing MsgLive")
	}
	if !snapshotSeen {
		t.Fatal("missing MsgSnapshot")
	}
	if recentRawCount != 5 {
		t.Fatalf("snapshot recent raw count = %d, want 5", recentRawCount)
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

	stepLine, _ := EncodeMsg(MsgLoopStepStart, WireLoopStepStart{
		RunID:      3,
		RunHexID:   "runhex",
		StepHexID:  "stephex",
		Cycle:      1,
		StepIndex:  0,
		Profile:    "reviewer",
		TotalSteps: 4,
	})
	b.broadcast(stepLine)
	startLine, _ := EncodeMsg(MsgStarted, WireStarted{
		SessionID: 42,
		TurnHexID: "turnhex",
	})
	b.broadcast(startLine)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var snapshot *WireSnapshot
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type != MsgSnapshot {
			continue
		}
		data, err := DecodeData[WireSnapshot](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireSnapshot]: %v", err)
		}
		snapshot = data
		break
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if snapshot == nil {
		t.Fatal("missing MsgSnapshot")
	}

	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var snapshot *WireSnapshot
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type != MsgSnapshot {
			continue
		}
		data, err := DecodeData[WireSnapshot](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireSnapshot]: %v", err)
		}
		snapshot = data
		break
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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
		// TotalSteps intentionally omitted.
	})

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var snapshot *WireSnapshot
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type != MsgSnapshot {
			continue
		}
		data, err := DecodeData[WireSnapshot](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireSnapshot]: %v", err)
		}
		snapshot = data
		break
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)

	if !sc.Scan() {
		t.Fatal("missing initial meta message")
	}
	metaMsg, err := DecodeMsg(sc.Bytes())
	if err != nil {
		t.Fatalf("DecodeMsg(meta): %v", err)
	}
	if metaMsg.Type != MsgMeta {
		t.Fatalf("first message type = %q, want %q", metaMsg.Type, MsgMeta)
	}

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

	b.broadcastTyped(MsgRaw, WireRaw{Data: "after", SessionID: 1})

	var snapshot *WireSnapshot
	liveSeen := false
	rawAfterLive := 0
	rawBeforeAfterLive := 0
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
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
			if data.Data == "before" {
				rawBeforeAfterLive++
				continue
			}
			if data.Data == "after" {
				rawAfterLive++
				goto done
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

done:
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
	}

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
	if rawBeforeAfterLive != 0 {
		t.Fatalf("pre-snapshot raw was replayed live %d times", rawBeforeAfterLive)
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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var snapshot *WireSnapshot
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type != MsgSnapshot {
			continue
		}
		data, err := DecodeData[WireSnapshot](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireSnapshot]: %v", err)
		}
		snapshot = data
		break
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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

func TestBroadcastSkipsClientQueueBeforeSnapshotBoundary(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()
	defer serverConn.Close()

	cc := newClientConn(serverConn, nil)
	cc.minSeq = 7

	b := &broadcaster{
		eventsFile: eventsFile,
		meta:       WireMeta{SessionID: 85},
		clients:    []*clientConn{cc},
		streamSeq:  5,
	}

	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 1, Data: "seq6"})
	select {
	case <-cc.sendCh:
		t.Fatal("unexpected queued event below snapshot boundary")
	default:
	}

	b.broadcastTyped(MsgRaw, WireRaw{SessionID: 1, Data: "seq7"})
	select {
	case line := <-cc.sendCh:
		msg, err := DecodeMsg(line)
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		data, err := DecodeData[WireRaw](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireRaw]: %v", err)
		}
		if data.Data != "seq7" {
			t.Fatalf("queued raw data = %q, want %q", data.Data, "seq7")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for queued event at snapshot boundary")
	}
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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var (
		snapshot *WireSnapshot
		order    []string
	)
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		order = append(order, msg.Type)
		switch msg.Type {
		case MsgSnapshot:
			data, err := DecodeData[WireSnapshot](msg)
			if err != nil {
				t.Fatalf("DecodeData[WireSnapshot]: %v", err)
			}
			snapshot = data
		case MsgDone:
			goto doneFinished
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

doneFinished:
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
	}

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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var snapshot *WireSnapshot
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type != MsgSnapshot {
			continue
		}
		data, err := DecodeData[WireSnapshot](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireSnapshot]: %v", err)
		}
		snapshot = data
		break
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	var snapshot *WireSnapshot
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type != MsgSnapshot {
			continue
		}
		data, err := DecodeData[WireSnapshot](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireSnapshot]: %v", err)
		}
		snapshot = data
		break
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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

	doneLine, err := EncodeMsg(MsgDone, WireDone{})
	if err != nil {
		t.Fatalf("EncodeMsg done: %v", err)
	}
	b.broadcast(doneLine)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	doneCount := 0
	liveSeen := false
	order := make([]string, 0, 8)
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		order = append(order, msg.Type)
		switch msg.Type {
		case MsgDone:
			doneCount++
			if doneCount >= 1 {
				goto done
			}
		case MsgLive:
			liveSeen = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}

done:
	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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
	clientDone := make(chan error, clients)
	for i := 0; i < clients; i++ {
		serverConn, clientConn := net.Pipe()
		go b.handleClient(serverConn, func() {})

		go func(c net.Conn) {
			defer c.Close()
			sc := bufio.NewScanner(c)
			sc.Buffer(make([]byte, 1024), 1024*1024)
			for sc.Scan() {
				msg, err := DecodeMsg(sc.Bytes())
				if err != nil {
					clientDone <- err
					return
				}
				if msg.Type == MsgLive {
					clientDone <- nil
					return
				}
			}
			clientDone <- sc.Err()
		}(clientConn)
	}

	for i := 0; i < 64; i++ {
		line, err := EncodeMsg(MsgRaw, WireRaw{Data: fmt.Sprintf("burst-%d", i)})
		if err != nil {
			t.Fatalf("EncodeMsg: %v", err)
		}
		b.broadcast(line)
	}

	for i := 0; i < clients; i++ {
		select {
		case err := <-clientDone:
			if err != nil {
				t.Fatalf("client %d error: %v", i, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("client %d timed out waiting for MsgLive", i)
		}
	}
}

func TestBroadcastDropsSlowClientWithoutBlocking(t *testing.T) {
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

	serverConn, peerConn := net.Pipe()
	defer peerConn.Close()

	var cc *clientConn
	cc = newClientConn(serverConn, func(err error) {
		b.removeClient(cc)
	})
	cc.startWriter()

	b.mu.Lock()
	b.clients = append(b.clients, cc)
	b.mu.Unlock()

	line, err := EncodeMsg(MsgRaw, WireRaw{Data: strings.Repeat("x", 512)})
	if err != nil {
		t.Fatalf("EncodeMsg raw: %v", err)
	}

	start := time.Now()
	for i := 0; i < 2000; i++ {
		b.broadcast(line)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("broadcast loop took %s; expected non-blocking behavior", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.clients)
		b.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("slow client was not dropped after queue backpressure")
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

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)

	// Wait until snapshot delivery is complete and the connection is live.
	liveSeen := false
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		if msg.Type == MsgLive {
			liveSeen = true
			break
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error waiting for live marker: %v", err)
	}
	if !liveSeen {
		t.Fatal("missing MsgLive")
	}

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
	if _, err := clientConn.Write(line); err != nil {
		t.Fatalf("writing control request: %v", err)
	}

	var got *WireControlResult
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				t.Fatalf("scanner error reading control response: %v", err)
			}
			t.Fatal("connection closed before control response")
		}
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			continue
		}
		if msg.Type != MsgControlResult {
			continue
		}
		got, err = DecodeData[WireControlResult](msg)
		if err != nil {
			t.Fatalf("DecodeData[WireControlResult]: %v", err)
		}
		break
	}
	if got == nil {
		t.Fatal("did not receive MsgControlResult")
	}
	if !got.OK {
		t.Fatalf("control response ok=false, error=%q", got.Error)
	}
	if got.SpawnID != 42 {
		t.Fatalf("spawn_id = %d, want 42", got.SpawnID)
	}

	_ = clientConn.Close()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("handleClient did not exit")
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
