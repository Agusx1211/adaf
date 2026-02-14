package session

import (
	"bufio"
	"math"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/runtui"
)

func TestStreamEventsForwardsLoopMessages(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	c := &Client{
		conn:    clientConn,
		scanner: bufio.NewScanner(clientConn),
	}

	eventCh := make(chan any, 16)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamEvents(eventCh, nil)
	}()

	writeWireMsg(t, serverConn, MsgLoopStepStart, WireLoopStepStart{
		RunID:      12,
		Cycle:      1,
		StepIndex:  2,
		Profile:    "reviewer",
		Turns:      3,
		TotalSteps: 5,
	})
	writeWireMsg(t, serverConn, MsgLoopStepEnd, WireLoopStepEnd{
		RunID:      12,
		Cycle:      1,
		StepIndex:  2,
		Profile:    "reviewer",
		TotalSteps: 5,
	})
	writeWireMsg(t, serverConn, MsgLoopDone, WireLoopDone{
		RunID:  12,
		Reason: "stopped",
	})
	writeWireMsg(t, serverConn, MsgDone, WireDone{})
	_ = serverConn.Close()

	var got []any
	for ev := range eventCh {
		got = append(got, ev)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("events = %d, want 3", len(got))
	}

	start, ok := got[0].(runtui.LoopStepStartMsg)
	if !ok {
		t.Fatalf("event[0] type = %T, want runtui.LoopStepStartMsg", got[0])
	}
	if start.RunID != 12 || start.Cycle != 1 || start.StepIndex != 2 || start.Profile != "reviewer" || start.Turns != 3 || start.TotalSteps != 5 {
		t.Fatalf("unexpected LoopStepStartMsg: %+v", start)
	}

	end, ok := got[1].(runtui.LoopStepEndMsg)
	if !ok {
		t.Fatalf("event[1] type = %T, want runtui.LoopStepEndMsg", got[1])
	}
	if end.RunID != 12 || end.Cycle != 1 || end.StepIndex != 2 || end.Profile != "reviewer" || end.TotalSteps != 5 {
		t.Fatalf("unexpected LoopStepEndMsg: %+v", end)
	}

	done, ok := got[2].(runtui.LoopDoneMsg)
	if !ok {
		t.Fatalf("event[2] type = %T, want runtui.LoopDoneMsg", got[2])
	}
	if done.RunID != 12 || done.Reason != "stopped" || done.Err != nil {
		t.Fatalf("unexpected LoopDoneMsg: %+v", done)
	}
}

func TestStreamEventsForwardsPromptMessages(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	c := &Client{
		conn:    clientConn,
		scanner: bufio.NewScanner(clientConn),
	}

	eventCh := make(chan any, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamEvents(eventCh, nil)
	}()

	writeWireMsg(t, serverConn, MsgPrompt, WirePrompt{
		SessionID:      4,
		TurnHexID:      "abc123",
		Prompt:         "hello prompt",
		IsResume:       true,
		Truncated:      true,
		OriginalLength: 4096,
	})
	writeWireMsg(t, serverConn, MsgDone, WireDone{})
	_ = serverConn.Close()

	var got []any
	for ev := range eventCh {
		got = append(got, ev)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}
	var msg runtui.AgentPromptMsg
	found := false
	for _, ev := range got {
		pm, ok := ev.(runtui.AgentPromptMsg)
		if !ok {
			continue
		}
		msg = pm
		found = true
		break
	}
	if !found {
		t.Fatalf("missing runtui.AgentPromptMsg in events: %#v", got)
	}
	if msg.SessionID != 4 || msg.TurnHexID != "abc123" || msg.Prompt != "hello prompt" {
		t.Fatalf("unexpected AgentPromptMsg: %+v", msg)
	}
	if !msg.IsResume || !msg.Truncated || msg.OriginalLength != 4096 {
		t.Fatalf("unexpected prompt flags: %+v", msg)
	}
}

func TestStreamEventsAppliesSnapshotAndRecentMessages(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	c := &Client{
		conn:    clientConn,
		scanner: bufio.NewScanner(clientConn),
	}

	eventCh := make(chan any, 16)
	errCh := make(chan error, 1)
	var liveCalls atomic.Int32
	go func() {
		errCh <- c.StreamEvents(eventCh, func() {
			liveCalls.Add(1)
		})
	}()

	recentPrompt := mustWireMsg(t, MsgPrompt, WirePrompt{
		SessionID: 7,
		TurnHexID: "turn-hex",
		Prompt:    "snapshot prompt",
	})
	recentRaw := mustWireMsg(t, MsgRaw, WireRaw{
		SessionID: 7,
		Data:      "snapshot output",
	})
	writeWireMsg(t, serverConn, MsgSnapshot, WireSnapshot{
		Loop: WireSnapshotLoop{
			RunID:      3,
			RunHexID:   "runhex",
			StepHexID:  "stephex",
			Cycle:      1,
			StepIndex:  2,
			Profile:    "reviewer",
			TotalSteps: 5,
		},
		Session: &WireSnapshotSession{
			SessionID:    7,
			TurnHexID:    "turn-hex",
			Agent:        "codex",
			Profile:      "reviewer",
			Model:        "gpt-5",
			InputTokens:  123,
			OutputTokens: 45,
			CostUSD:      0.0042,
			NumTurns:     3,
			Status:       "running",
			Action:       "responding",
			StartedAt:    time.Now().UTC().Add(-10 * time.Second),
		},
		Spawns: []WireSpawnInfo{
			{ID: 9, ParentTurnID: 7, Profile: "devstral2", Status: "running"},
		},
		Recent: []WireMsg{recentPrompt, recentRaw},
	})
	writeWireMsg(t, serverConn, MsgLive, nil)
	writeWireMsg(t, serverConn, MsgRaw, WireRaw{
		SessionID: 7,
		Data:      "live output",
	})
	writeWireMsg(t, serverConn, MsgDone, WireDone{})
	_ = serverConn.Close()

	var got []any
	for ev := range eventCh {
		got = append(got, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("events = %d, want 5", len(got))
	}

	snap, ok := got[0].(runtui.SessionSnapshotMsg)
	if !ok {
		t.Fatalf("event[0] type = %T, want runtui.SessionSnapshotMsg", got[0])
	}
	if snap.Loop.RunID != 3 || snap.Loop.StepIndex != 2 || snap.Loop.TotalSteps != 5 {
		t.Fatalf("unexpected loop snapshot: %+v", snap.Loop)
	}
	if snap.Session == nil || snap.Session.SessionID != 7 || snap.Session.Agent != "codex" {
		t.Fatalf("unexpected session snapshot: %+v", snap.Session)
	}
	if snap.Session.InputTokens != 123 || snap.Session.OutputTokens != 45 || snap.Session.NumTurns != 3 {
		t.Fatalf("unexpected usage snapshot: %+v", snap.Session)
	}
	if math.Abs(snap.Session.CostUSD-0.0042) > 1e-9 {
		t.Fatalf("unexpected cost snapshot: %+v", snap.Session)
	}
	if len(snap.Spawns) != 1 || snap.Spawns[0].ID != 9 {
		t.Fatalf("unexpected spawns snapshot: %+v", snap.Spawns)
	}

	prompt, ok := got[1].(runtui.AgentPromptMsg)
	if !ok {
		t.Fatalf("event[1] type = %T, want runtui.AgentPromptMsg", got[1])
	}
	if prompt.Prompt != "snapshot prompt" || prompt.SessionID != 7 {
		t.Fatalf("unexpected AgentPromptMsg: %+v", prompt)
	}

	raw, ok := got[2].(runtui.AgentRawOutputMsg)
	if !ok {
		t.Fatalf("event[2] type = %T, want runtui.AgentRawOutputMsg", got[2])
	}
	if raw.Data != "snapshot output" || raw.SessionID != 7 {
		t.Fatalf("unexpected AgentRawOutputMsg: %+v", raw)
	}

	liveRaw, ok := got[3].(runtui.AgentRawOutputMsg)
	if !ok {
		t.Fatalf("event[3] type = %T, want runtui.AgentRawOutputMsg", got[3])
	}
	if liveRaw.Data != "live output" || liveRaw.SessionID != 7 {
		t.Fatalf("unexpected live AgentRawOutputMsg: %+v", liveRaw)
	}
	if liveCalls.Load() != 1 {
		t.Fatalf("isLive callback count = %d, want 1", liveCalls.Load())
	}

	if _, ok := got[4].(runtui.AgentLoopDoneMsg); !ok {
		t.Fatalf("event[4] type = %T, want runtui.AgentLoopDoneMsg", got[4])
	}
}

func TestStreamEventsIgnoresUnsupportedSnapshotRecentTypes(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()

	c := &Client{
		conn:    clientConn,
		scanner: bufio.NewScanner(clientConn),
	}

	eventCh := make(chan any, 16)
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.StreamEvents(eventCh, nil)
	}()

	recentLoopDone := mustWireMsg(t, MsgLoopDone, WireLoopDone{RunID: 3, Reason: "stopped"})
	recentRaw := mustWireMsg(t, MsgRaw, WireRaw{SessionID: 7, Data: "snapshot output"})
	writeWireMsg(t, serverConn, MsgSnapshot, WireSnapshot{
		Recent: []WireMsg{recentLoopDone, recentRaw},
	})
	writeWireMsg(t, serverConn, MsgLive, nil)
	writeWireMsg(t, serverConn, MsgDone, WireDone{})
	_ = serverConn.Close()

	var got []any
	for ev := range eventCh {
		got = append(got, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3", len(got))
	}
	if _, ok := got[0].(runtui.SessionSnapshotMsg); !ok {
		t.Fatalf("event[0] type = %T, want runtui.SessionSnapshotMsg", got[0])
	}
	if raw, ok := got[1].(runtui.AgentRawOutputMsg); !ok {
		t.Fatalf("event[1] type = %T, want runtui.AgentRawOutputMsg", got[1])
	} else if raw.Data != "snapshot output" {
		t.Fatalf("event[1] raw data = %q, want %q", raw.Data, "snapshot output")
	}
	if _, ok := got[2].(runtui.AgentLoopDoneMsg); !ok {
		t.Fatalf("event[2] type = %T, want runtui.AgentLoopDoneMsg", got[2])
	}
	for i, ev := range got {
		if _, ok := ev.(runtui.LoopDoneMsg); ok {
			t.Fatalf("event[%d] unexpectedly contains runtui.LoopDoneMsg from snapshot recent", i)
		}
	}
}

func writeWireMsg(t *testing.T, conn net.Conn, msgType string, payload any) {
	t.Helper()
	line, err := EncodeMsg(msgType, payload)
	if err != nil {
		t.Fatalf("EncodeMsg(%q): %v", msgType, err)
	}
	if _, err := conn.Write(line); err != nil {
		t.Fatalf("conn.Write(%q): %v", msgType, err)
	}
}

func mustWireMsg(t *testing.T, msgType string, payload any) WireMsg {
	t.Helper()
	line, err := EncodeMsg(msgType, payload)
	if err != nil {
		t.Fatalf("EncodeMsg(%q): %v", msgType, err)
	}
	msg, err := DecodeMsg(line)
	if err != nil {
		t.Fatalf("DecodeMsg(%q): %v", msgType, err)
	}
	return *msg
}
