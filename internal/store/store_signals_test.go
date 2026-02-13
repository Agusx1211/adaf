package store

import (
	"testing"
	"time"
)

func TestWaitAndInterruptSignalsNotifyChannels(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(ProjectConfig{Name: "signal-test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	waitTurn := 101
	waitCh := s.WaitSignalChan(waitTurn)
	if err := s.SignalWait(waitTurn); err != nil {
		t.Fatalf("SignalWait: %v", err)
	}
	select {
	case <-waitCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for wait signal")
	}
	if !s.IsWaiting(waitTurn) {
		t.Fatalf("IsWaiting(%d) = false, want true", waitTurn)
	}
	if err := s.ClearWait(waitTurn); err != nil {
		t.Fatalf("ClearWait: %v", err)
	}
	s.signalMu.Lock()
	if _, ok := s.waitSignals[waitTurn]; ok {
		s.signalMu.Unlock()
		t.Fatalf("wait signal channel for turn %d should be released", waitTurn)
	}
	s.signalMu.Unlock()

	interruptID := 202
	interruptCh := s.InterruptSignalChan(interruptID)
	if err := s.SignalInterrupt(interruptID, "please rebase and retry"); err != nil {
		t.Fatalf("SignalInterrupt: %v", err)
	}
	select {
	case got := <-interruptCh:
		if got != "please rebase and retry" {
			t.Fatalf("interrupt msg = %q, want %q", got, "please rebase and retry")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for interrupt signal")
	}
	if got := s.CheckInterrupt(interruptID); got != "please rebase and retry" {
		t.Fatalf("CheckInterrupt = %q, want %q", got, "please rebase and retry")
	}
	if err := s.ClearInterrupt(interruptID); err != nil {
		t.Fatalf("ClearInterrupt: %v", err)
	}
	if got := s.CheckInterrupt(interruptID); got != "" {
		t.Fatalf("CheckInterrupt after clear = %q, want empty", got)
	}
	s.ReleaseInterruptSignal(interruptID)
	s.signalMu.Lock()
	if _, ok := s.interruptSignals[interruptID]; ok {
		s.signalMu.Unlock()
		t.Fatalf("interrupt signal channel for id %d should be released", interruptID)
	}
	s.signalMu.Unlock()
}
