package tui

import (
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/store"
)

func TestUpdateRunningLoopStepStartBindsSessionMode(t *testing.T) {
	m := AppModel{
		state:    stateRunning,
		runModel: runtui.NewModel("proj", nil, "", "", make(chan any, 1), nil),
	}

	if got := m.runModel.SessionMode(); got != 0 {
		t.Fatalf("initial session mode = %d, want 0", got)
	}

	updated, _ := m.updateRunning(runtui.LoopStepStartMsg{
		RunID:     42,
		Cycle:     0,
		StepIndex: 0,
		Profile:   "dev",
		Turns:     1,
	})

	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if sid := got.runModel.SessionMode(); sid != 42 {
		t.Fatalf("session mode = %d, want 42", sid)
	}
}

func TestStartBackgroundEventDrainConsumesMessages(t *testing.T) {
	ch := make(chan any)
	startBackgroundEventDrain(ch)

	sent := make(chan struct{})
	go func() {
		ch <- runtui.AgentRawOutputMsg{Data: "hello"}
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatalf("send blocked; background drain not consuming events")
	}

	close(ch)
}

func TestUpdateRunningDetachStartsBackgroundDrain(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	ch := make(chan any)
	m := AppModel{
		store:      s,
		state:      stateRunning,
		runModel:   runtui.NewModel("proj", nil, "", "", make(chan any, 1), nil),
		runEventCh: ch,
	}

	updated, _ := m.updateRunning(runtui.DetachMsg{SessionID: 9})
	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if got.state != stateSelector {
		t.Fatalf("state = %v, want stateSelector", got.state)
	}
	if got.runEventCh != nil {
		t.Fatalf("runEventCh = %v, want nil", got.runEventCh)
	}

	sent := make(chan struct{})
	go func() {
		ch <- runtui.AgentRawOutputMsg{Data: "after-detach"}
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatalf("send blocked after detach; expected background drain")
	}

	close(ch)
}
