package recording

import (
	"io"
	"sync"
	"time"

	"github.com/agusx1211/adaf/internal/store"
)

// Recorder captures all I/O events for a turn and persists them to the store.
type Recorder struct {
	TurnID int
	Store  *store.Store

	mu     sync.Mutex
	events []store.RecordingEvent
}

// New creates a new Recorder for the given turn.
func New(turnID int, s *store.Store) *Recorder {
	return &Recorder{
		TurnID: turnID,
		Store:  s,
	}
}

// RecordStdout records a stdout data chunk.
func (r *Recorder) RecordStdout(data string) {
	r.record("stdout", data)
}

// RecordStderr records a stderr data chunk.
func (r *Recorder) RecordStderr(data string) {
	r.record("stderr", data)
}

// RecordStdin records a stdin data chunk.
func (r *Recorder) RecordStdin(data string) {
	r.record("stdin", data)
}

// RecordMeta records a metadata key-value pair as a "meta" event.
// The data is stored as "key=value".
func (r *Recorder) RecordMeta(key, value string) {
	r.record("meta", key+"="+value)
}

// RecordStream records a raw NDJSON line from Claude's stream-json output.
func (r *Recorder) RecordStream(rawJSON string) {
	r.record("claude_stream", rawJSON)
}

// record appends an event both to the in-memory buffer and to the store
// (via AppendRecordingEvent for streaming persistence).
func (r *Recorder) record(eventType, data string) {
	event := store.RecordingEvent{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Data:      data,
	}

	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()

	// Best-effort append to persistent store; errors are non-fatal
	// so we don't interrupt the running agent.
	_ = r.Store.AppendRecordingEvent(r.TurnID, event)
}

// Events returns a snapshot of all recorded events.
func (r *Recorder) Events() []store.RecordingEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]store.RecordingEvent, len(r.events))
	copy(cp, r.events)
	return cp
}

// Flush writes the full turn recording to the store.
func (r *Recorder) Flush() error {
	r.mu.Lock()
	events := make([]store.RecordingEvent, len(r.events))
	copy(events, r.events)
	r.mu.Unlock()

	rec := &store.TurnRecording{
		TurnID: r.TurnID,
		Events: events,
	}
	return r.Store.SaveRecording(rec)
}

// WrapWriter returns an io.Writer that writes to the underlying writer w
// and simultaneously records every write as an event of the given eventType.
// This allows streaming output to both the terminal and the recorder.
func (r *Recorder) WrapWriter(w io.Writer, eventType string) io.Writer {
	return &recordingWriter{
		recorder:  r,
		inner:     w,
		eventType: eventType,
	}
}

// recordingWriter is an io.Writer that tees all writes to a Recorder.
type recordingWriter struct {
	recorder  *Recorder
	inner     io.Writer
	eventType string
}

func (rw *recordingWriter) Write(p []byte) (int, error) {
	// Record the data before writing to the inner writer.
	switch rw.eventType {
	case "stdout":
		rw.recorder.RecordStdout(string(p))
	case "stderr":
		rw.recorder.RecordStderr(string(p))
	case "stdin":
		rw.recorder.RecordStdin(string(p))
	default:
		rw.recorder.record(rw.eventType, string(p))
	}

	// Write to the underlying writer (e.g. os.Stdout).
	if rw.inner != nil {
		return rw.inner.Write(p)
	}
	return len(p), nil
}
