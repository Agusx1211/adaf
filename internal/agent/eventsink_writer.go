package agent

import (
	"sync/atomic"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/eventq"
	"github.com/agusx1211/adaf/internal/stream"
)

// eventSinkWriter forwards raw output chunks to the configured EventSink.
type eventSinkWriter struct {
	sink        chan<- stream.RawEvent
	turnID      int
	prefix      string
	dropCounter atomic.Uint64
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
	ok := eventq.Offer(w.sink, stream.RawEvent{
		Text:   w.prefix + string(p),
		TurnID: w.turnID,
	})
	if !ok {
		dropped := w.dropCounter.Add(1)
		if dropped == 1 || dropped%100 == 0 {
			debug.LogKV("agent.eventsink", "dropping raw output event due to backpressure", "turn_id", w.turnID, "dropped", dropped)
		}
	}
	return len(p), nil
}
