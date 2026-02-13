package session

import (
	"bufio"
	"net"
	"testing"

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
