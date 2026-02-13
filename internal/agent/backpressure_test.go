package agent

import (
	"io"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

func TestEventSinkWriterDoesNotBlockWhenSinkIsFull(t *testing.T) {
	sink := make(chan stream.RawEvent, 1)
	sink <- stream.RawEvent{Text: "occupied"}

	w := newEventSinkWriter(sink, 77, "")
	done := make(chan struct{})
	go func() {
		_, _ = w.Write([]byte("hello"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("eventSinkWriter.Write blocked on full sink")
	}
}

func TestRunStreamLoopDoesNotBlockWhenSinkIsFull(t *testing.T) {
	sink := make(chan stream.RawEvent, 1)
	sink <- stream.RawEvent{Text: "occupied"}

	events := make(chan stream.RawEvent, 1)
	events <- stream.RawEvent{
		Raw: []byte(`{"type":"assistant"}`),
		Parsed: stream.ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &stream.AssistantMessage{
				Content: []stream.ContentBlock{
					{Type: "text", Text: "hello"},
				},
			},
		},
	}
	close(events)

	cfg := Config{
		EventSink: sink,
		TurnID:    99,
	}
	rec := newBackpressureTestRecorder(t)
	done := make(chan struct{})
	go func() {
		runStreamLoop(cfg, events, rec, time.Now(), io.Discard)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("runStreamLoop blocked on full sink")
	}
}

func newBackpressureTestRecorder(t *testing.T) *recording.Recorder {
	t.Helper()

	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	return recording.New(1, s)
}
