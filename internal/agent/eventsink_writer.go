package agent

import "github.com/agusx1211/adaf/internal/stream"

// eventSinkWriter forwards raw output chunks to the configured EventSink.
type eventSinkWriter struct {
	sink   chan<- stream.RawEvent
	turnID int
	prefix string
}

func newEventSinkWriter(sink chan<- stream.RawEvent, turnID int, prefix string) *eventSinkWriter {
	if sink == nil {
		return nil
	}
	return &eventSinkWriter{
		sink:   sink,
		turnID: turnID,
		prefix: prefix,
	}
}

func (w *eventSinkWriter) Write(p []byte) (int, error) {
	if w == nil || w.sink == nil || len(p) == 0 {
		return len(p), nil
	}
	w.sink <- stream.RawEvent{
		Text:   w.prefix + string(p),
		TurnID: w.turnID,
	}
	return len(p), nil
}
