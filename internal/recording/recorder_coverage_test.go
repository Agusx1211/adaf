package recording

import (
	"bytes"
	"sync"
	"testing"
)

func TestRecordStdoutAddsEvent(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(1, s)
	r.RecordStdout("hello world")

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	if events[0].Type != "stdout" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "stdout")
	}
	if events[0].Data != "hello world" {
		t.Fatalf("event data = %q, want %q", events[0].Data, "hello world")
	}
}

func TestRecordStderrAddsEvent(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(2, s)
	r.RecordStderr("error msg")

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	if events[0].Type != "stderr" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "stderr")
	}
	if events[0].Data != "error msg" {
		t.Fatalf("event data = %q, want %q", events[0].Data, "error msg")
	}
}

func TestRecordStdinAddsEvent(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(3, s)
	r.RecordStdin("input data")

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	if events[0].Type != "stdin" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "stdin")
	}
}

func TestRecordMetaFormatsKeyValue(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(4, s)
	r.RecordMeta("agent", "claude")
	r.RecordMeta("turn", "5")

	events := r.Events()
	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2", len(events))
	}
	if events[0].Data != "agent=claude" {
		t.Fatalf("meta data = %q, want %q", events[0].Data, "agent=claude")
	}
	if events[1].Data != "turn=5" {
		t.Fatalf("meta data = %q, want %q", events[1].Data, "turn=5")
	}
}

func TestRecordStreamUsesClaudeStreamType(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(5, s)
	r.RecordStream(`{"type":"assistant","message":{"content":[]}}`)

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	if events[0].Type != "claude_stream" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "claude_stream")
	}
}

func TestEventsReturnsSnapshotCopy(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(6, s)
	r.RecordStdout("first")

	snapshot := r.Events()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot count = %d, want 1", len(snapshot))
	}

	// Adding more events should NOT affect the snapshot.
	r.RecordStdout("second")
	if len(snapshot) != 1 {
		t.Fatalf("snapshot count changed to %d after subsequent record", len(snapshot))
	}

	// New snapshot should have both events.
	snapshot2 := r.Events()
	if len(snapshot2) != 2 {
		t.Fatalf("second snapshot count = %d, want 2", len(snapshot2))
	}
}

func TestEventOrdering(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(7, s)

	r.RecordMeta("agent", "codex")
	r.RecordStdin("prompt")
	r.RecordStdout("output")
	r.RecordStderr("warning")
	r.RecordStream(`{"type":"done"}`)

	events := r.Events()
	if len(events) != 5 {
		t.Fatalf("events count = %d, want 5", len(events))
	}
	wantTypes := []string{"meta", "stdin", "stdout", "stderr", "claude_stream"}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Fatalf("event[%d].Type = %q, want %q", i, events[i].Type, want)
		}
	}
}

func TestFlushPersistsToStore(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(10, s)
	r.RecordMeta("agent", "test")
	r.RecordStdout("hello")

	if err := r.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	rec, err := s.LoadRecording(10)
	if err != nil {
		t.Fatalf("LoadRecording() error = %v", err)
	}
	if len(rec.Events) != 2 {
		t.Fatalf("persisted events = %d, want 2", len(rec.Events))
	}
	if rec.TurnID != 10 {
		t.Fatalf("persisted TurnID = %d, want 10", rec.TurnID)
	}
}

func TestFlushWithNoEventsSucceeds(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(11, s)

	if err := r.Flush(); err != nil {
		t.Fatalf("Flush() with no events error = %v", err)
	}

	rec, err := s.LoadRecording(11)
	if err != nil {
		t.Fatalf("LoadRecording() error = %v", err)
	}
	if len(rec.Events) != 0 {
		t.Fatalf("persisted events = %d, want 0", len(rec.Events))
	}
}

func TestWrapWriterStderrType(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(12, s)

	var buf bytes.Buffer
	w := r.WrapWriter(&buf, "stderr")
	n, err := w.Write([]byte("error output"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len("error output") {
		t.Fatalf("Write() count = %d, want %d", n, len("error output"))
	}
	if buf.String() != "error output" {
		t.Fatalf("inner writer = %q, want %q", buf.String(), "error output")
	}

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Type != "stderr" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "stderr")
	}
}

func TestWrapWriterStdinType(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(13, s)

	w := r.WrapWriter(nil, "stdin")
	_, err := w.Write([]byte("user input"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Type != "stdin" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "stdin")
	}
}

func TestWrapWriterCustomType(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(14, s)

	w := r.WrapWriter(nil, "custom_event")
	_, _ = w.Write([]byte("data"))

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Type != "custom_event" {
		t.Fatalf("event type = %q, want %q", events[0].Type, "custom_event")
	}
}

func TestConcurrentRecording(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(15, s)

	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			r.RecordStdout("concurrent")
		}()
	}
	wg.Wait()

	events := r.Events()
	if len(events) != n {
		t.Fatalf("events = %d, want %d", len(events), n)
	}
}

func TestMultipleFlushesAreIdempotent(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(16, s)
	r.RecordStdout("data")

	if err := r.Flush(); err != nil {
		t.Fatalf("first Flush() error = %v", err)
	}
	if err := r.Flush(); err != nil {
		t.Fatalf("second Flush() error = %v", err)
	}

	rec, err := s.LoadRecording(16)
	if err != nil {
		t.Fatalf("LoadRecording() error = %v", err)
	}
	if len(rec.Events) != 1 {
		t.Fatalf("events after double flush = %d, want 1", len(rec.Events))
	}
}

func TestTimestampsAreSet(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(17, s)
	r.RecordStdout("timestamped")

	events := r.Events()
	if events[0].Timestamp.IsZero() {
		t.Fatal("event timestamp is zero")
	}
}

func TestRecorderWithMultipleTurnIDs(t *testing.T) {
	s := newRecorderTestStore(t)

	r1 := New(100, s)
	r1.RecordStdout("turn 100")
	if err := r1.Flush(); err != nil {
		t.Fatalf("r1.Flush() error = %v", err)
	}

	r2 := New(101, s)
	r2.RecordStdout("turn 101")
	if err := r2.Flush(); err != nil {
		t.Fatalf("r2.Flush() error = %v", err)
	}

	rec1, err := s.LoadRecording(100)
	if err != nil {
		t.Fatalf("LoadRecording(100) error = %v", err)
	}
	rec2, err := s.LoadRecording(101)
	if err != nil {
		t.Fatalf("LoadRecording(101) error = %v", err)
	}
	if len(rec1.Events) != 1 || rec1.Events[0].Data != "turn 100" {
		t.Fatalf("rec1 unexpected: %v", rec1.Events)
	}
	if len(rec2.Events) != 1 || rec2.Events[0].Data != "turn 101" {
		t.Fatalf("rec2 unexpected: %v", rec2.Events)
	}
}
