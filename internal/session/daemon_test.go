package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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

	for _, entry := range b.buffered {
		msg, err := DecodeMsg(entry.Line)
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

func TestHandleClientReplaysFromFileWhenBufferTrimmed(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	eventsFile, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer eventsFile.Close()

	b := &broadcaster{
		eventsFile:      eventsFile,
		meta:            WireMeta{SessionID: 7},
		maxReplayEvents: 2,
	}

	for i := 0; i < 5; i++ {
		line, err := EncodeMsg(MsgRaw, WireRaw{Data: fmt.Sprintf("line-%d", i+1)})
		if err != nil {
			t.Fatalf("EncodeMsg raw: %v", err)
		}
		b.broadcast(line)
	}

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	doneCh := make(chan struct{})
	go func() {
		b.handleClient(serverConn, func() {})
		close(doneCh)
	}()

	sc := bufio.NewScanner(clientConn)
	sc.Buffer(make([]byte, 1024), 1024*1024)
	rawCount := 0
	liveSeen := false
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		switch msg.Type {
		case MsgRaw:
			rawCount++
		case MsgLive:
			liveSeen = true
			goto done
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

	if !liveSeen {
		t.Fatal("missing MsgLive")
	}
	if rawCount != 5 {
		t.Fatalf("raw replay count = %d, want 5", rawCount)
	}
}

func TestHandleClientDoesNotDuplicateDoneAfterMarkDone(t *testing.T) {
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
	b.markDone()

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
	for sc.Scan() {
		msg, err := DecodeMsg(sc.Bytes())
		if err != nil {
			t.Fatalf("DecodeMsg: %v", err)
		}
		switch msg.Type {
		case MsgDone:
			doneCount++
		case MsgLive:
			liveSeen = true
			goto done
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

	if !liveSeen {
		t.Fatal("missing MsgLive")
	}
	if doneCount != 1 {
		t.Fatalf("done message count = %d, want 1", doneCount)
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
		eventsFile:      eventsFile,
		meta:            WireMeta{SessionID: 9},
		maxReplayEvents: 8,
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
