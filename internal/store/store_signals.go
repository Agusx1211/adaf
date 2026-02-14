// store_signals.go contains wait and interrupt signal management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *Store) waitSignalChan(turnID int) chan struct{} {
	s.signalMu.Lock()
	defer s.signalMu.Unlock()
	ch := s.waitSignals[turnID]
	if ch == nil {
		ch = make(chan struct{}, 1)
		s.waitSignals[turnID] = ch
	}
	return ch
}

// WaitSignalChan returns a process-local notification channel that emits when
// SignalWait is called for the given turn ID.

func (s *Store) WaitSignalChan(turnID int) <-chan struct{} {
	return s.waitSignalChan(turnID)
}

func (s *Store) notifyWaitSignal(turnID int) {
	ch := s.waitSignalChan(turnID)
	select {
	case ch <- struct{}{}:
	default:
	}
}

// ReleaseWaitSignal removes the process-local wait notification channel for a turn.
// This is a memory hygiene helper and does not touch on-disk signal files.

func (s *Store) ReleaseWaitSignal(turnID int) {
	s.signalMu.Lock()
	delete(s.waitSignals, turnID)
	s.signalMu.Unlock()
}

// SignalWait creates a wait signal file for a turn.
// This indicates the agent wants to pause and resume when spawns complete.

func (s *Store) SignalWait(turnID int) error {
	dir := filepath.Join(s.root, "waits")
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d", turnID)), []byte("waiting"), 0644); err != nil {
		return err
	}
	s.notifyWaitSignal(turnID)
	return nil
}

// IsWaiting checks if a wait signal exists for a turn.

func (s *Store) IsWaiting(turnID int) bool {
	_, err := os.Stat(filepath.Join(s.root, "waits", fmt.Sprintf("%d", turnID)))
	return err == nil
}

// ClearWait removes the wait signal for a turn.

func (s *Store) ClearWait(turnID int) error {
	err := os.Remove(filepath.Join(s.root, "waits", fmt.Sprintf("%d", turnID)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	s.ReleaseWaitSignal(turnID)
	return nil
}

// --- Interrupt Signal ---

func (s *Store) interruptSignalChan(turnID int) chan string {
	s.signalMu.Lock()
	defer s.signalMu.Unlock()
	ch := s.interruptSignals[turnID]
	if ch == nil {
		ch = make(chan string, 8)
		s.interruptSignals[turnID] = ch
	}
	return ch
}

// InterruptSignalChan returns a process-local notification channel that emits
// interrupt messages for the given spawn/turn ID.

func (s *Store) InterruptSignalChan(turnID int) <-chan string {
	return s.interruptSignalChan(turnID)
}

func (s *Store) notifyInterruptSignal(turnID int, message string) {
	ch := s.interruptSignalChan(turnID)
	select {
	case ch <- message:
	default:
	}
}

// ReleaseInterruptSignal removes the process-local interrupt notification channel
// for a spawn/turn ID. This is a memory hygiene helper and does not touch on-disk
// signal files.

func (s *Store) ReleaseInterruptSignal(turnID int) {
	s.signalMu.Lock()
	delete(s.interruptSignals, turnID)
	s.signalMu.Unlock()
}

// SignalInterrupt creates an interrupt signal file for a turn.

func (s *Store) SignalInterrupt(turnID int, message string) error {
	dir := filepath.Join(s.root, "interrupts")
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d", turnID)), []byte(message), 0644); err != nil {
		return err
	}
	s.notifyInterruptSignal(turnID, message)
	return nil
}

// CheckInterrupt checks for and returns an interrupt message for a turn.
// Returns empty string if no interrupt is pending.

func (s *Store) CheckInterrupt(turnID int) string {
	path := filepath.Join(s.root, "interrupts", fmt.Sprintf("%d", turnID))
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// ClearInterrupt removes the interrupt signal for a spawn.

func (s *Store) ClearInterrupt(spawnID int) error {
	return os.Remove(filepath.Join(s.root, "interrupts", fmt.Sprintf("%d", spawnID)))
}
