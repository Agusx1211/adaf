package recording

import (
	"bytes"
	"testing"

	"github.com/agusx1211/adaf/internal/store"
)

func TestRecorderRecordsAndFlushesEvents(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(42, s)

	r.RecordMeta("agent", "codex")
	r.RecordStdout("hello")
	r.RecordStderr("warn")
	r.RecordStdin("input")
	r.RecordStream(`{"type":"assistant"}`)

	var out bytes.Buffer
	w := r.WrapWriter(&out, "stdout")
	if _, err := w.Write([]byte("stream")); err != nil {
		t.Fatalf("write wrapped stdout: %v", err)
	}
	if got := out.String(); got != "stream" {
		t.Fatalf("wrapped writer output = %q, want %q", got, "stream")
	}

	events := r.Events()
	if len(events) != 6 {
		t.Fatalf("events count = %d, want 6", len(events))
	}
	if events[0].Type != "meta" || events[0].Data != "agent=codex" {
		t.Fatalf("first event = (%q, %q), want meta agent=codex", events[0].Type, events[0].Data)
	}
	if events[4].Type != "claude_stream" {
		t.Fatalf("stream event type = %q, want claude_stream", events[4].Type)
	}
	if events[5].Type != "stdout" || events[5].Data != "stream" {
		t.Fatalf("wrapped event = (%q, %q), want stdout stream", events[5].Type, events[5].Data)
	}

	if err := r.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	rec, err := s.LoadRecording(42)
	if err != nil {
		t.Fatalf("LoadRecording: %v", err)
	}
	if len(rec.Events) != len(events) {
		t.Fatalf("recording events count = %d, want %d", len(rec.Events), len(events))
	}
}

func TestRecorderWrapWriterWithNilInnerStillRecords(t *testing.T) {
	s := newRecorderTestStore(t)
	r := New(7, s)

	w := r.WrapWriter(nil, "stderr")
	n, err := w.Write([]byte("line"))
	if err != nil {
		t.Fatalf("write with nil inner: %v", err)
	}
	if n != len("line") {
		t.Fatalf("write count = %d, want %d", n, len("line"))
	}

	events := r.Events()
	if len(events) != 1 {
		t.Fatalf("events count = %d, want 1", len(events))
	}
	if events[0].Type != "stderr" || events[0].Data != "line" {
		t.Fatalf("event = (%q, %q), want stderr line", events[0].Type, events[0].Data)
	}
}

func newRecorderTestStore(t *testing.T) *store.Store {
	t.Helper()

	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "recording-test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	return s
}
