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
