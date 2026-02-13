package orchestrator

import (
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/runtui"
)

func TestEmitEventDoesNotBlockWhenChannelIsFull(t *testing.T) {
	o := &Orchestrator{}
	ch := make(chan any, 1)
	ch <- struct{}{}
	o.SetEventCh(ch)

	done := make(chan struct{})
	go func() {
		o.emitEvent("agent_finished", runtui.AgentFinishedMsg{SessionID: -1})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("emitEvent blocked on full event channel")
	}
}

func TestEmitEventHandlesClosedChannelWithoutPanic(t *testing.T) {
	o := &Orchestrator{}
	ch := make(chan any, 1)
	o.SetEventCh(ch)
	close(ch)

	if sent := o.emitEvent("agent_finished", runtui.AgentFinishedMsg{SessionID: -1}); sent {
		t.Fatalf("emitEvent on closed channel returned true, want false")
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.eventCh != nil {
		t.Fatalf("eventCh should be reset to nil after closed-channel send panic")
	}
}
