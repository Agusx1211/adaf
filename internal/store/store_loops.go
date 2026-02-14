// store_loops.go contains loop run and loop message management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) loopRunPath(id int) string {
	return filepath.Join(s.root, "loopruns", fmt.Sprintf("%d.json", id))
}

// loopRunDir returns the directory for a loop run's associated data.

func (s *Store) loopRunDir(id int) string {
	return filepath.Join(s.root, "loopruns", fmt.Sprintf("%d", id))
}

// CreateLoopRun persists a new loop run with an auto-assigned ID.

func (s *Store) CreateLoopRun(run *LoopRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.root, "loopruns")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := s.stopRunningLoopRunsLocked(dir); err != nil {
		return err
	}

	run.ID = s.nextID(dir)
	run.StartedAt = time.Now().UTC()
	if run.Status == "" {
		run.Status = "running"
	}
	if run.StepLastSeenMsg == nil {
		run.StepLastSeenMsg = make(map[int]int)
	}
	return s.writeJSONLocked(s.loopRunPath(run.ID), run)
}

// GetLoopRun loads a single loop run by ID.

func (s *Store) GetLoopRun(id int) (*LoopRun, error) {
	var run LoopRun
	if err := s.readJSONLocked(s.loopRunPath(id), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// UpdateLoopRun persists changes to a loop run.

func (s *Store) UpdateLoopRun(run *LoopRun) error {
	return s.writeJSONLocked(s.loopRunPath(run.ID), run)
}

// ActiveLoopRun finds the currently running loop run, if any.

func (s *Store) ActiveLoopRun() (*LoopRun, error) {
	dir := filepath.Join(s.root, "loopruns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var latest *LoopRun
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var run LoopRun
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &run); err != nil {
			continue
		}
		if run.Status == "running" {
			if latest == nil || run.ID > latest.ID {
				cp := run
				latest = &cp
			}
		}
	}
	return latest, nil
}

// ListLoopRuns returns all loop runs, sorted by ID.

func (s *Store) ListLoopRuns() ([]LoopRun, error) {
	dir := filepath.Join(s.root, "loopruns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runs []LoopRun
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var run LoopRun
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs, nil
}

func (s *Store) stopRunningLoopRunsLocked(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	stoppedAt := time.Now().UTC()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		var run LoopRun
		if err := s.readJSONLocked(path, &run); err != nil {
			continue
		}
		if run.Status != "running" {
			continue
		}

		run.Status = "stopped"
		run.StoppedAt = stoppedAt
		if err := s.writeJSONLocked(path, &run); err != nil {
			return err
		}
	}

	return nil
}

// --- Loop Messages ---

// loopMessagesDir returns the directory for a loop run's messages.

func (s *Store) loopMessagesDir(runID int) string {
	return filepath.Join(s.loopRunDir(runID), "messages")
}

// CreateLoopMessage persists a new loop message with an auto-assigned ID.

func (s *Store) CreateLoopMessage(msg *LoopMessage) error {
	dir := s.loopMessagesDir(msg.RunID)
	os.MkdirAll(dir, 0755)

	s.mu.Lock()
	msg.ID = s.nextID(dir)
	s.mu.Unlock()

	msg.CreatedAt = time.Now().UTC()
	return s.writeJSONLocked(filepath.Join(dir, fmt.Sprintf("%d.json", msg.ID)), msg)
}

// ListLoopMessages returns all messages for a loop run, sorted by ID.

func (s *Store) ListLoopMessages(runID int) ([]LoopMessage, error) {
	dir := s.loopMessagesDir(runID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var msgs []LoopMessage
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var msg LoopMessage
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })
	return msgs, nil
}

// --- Loop Stop Signal ---

// SignalLoopStop creates a stop signal file for a loop run.

func (s *Store) SignalLoopStop(runID int) error {
	dir := s.loopRunDir(runID)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(filepath.Join(dir, "stop"), []byte("stop"), 0644)
}

// IsLoopStopped checks if a stop signal exists for a loop run.

func (s *Store) IsLoopStopped(runID int) bool {
	_, err := os.Stat(filepath.Join(s.loopRunDir(runID), "stop"))
	return err == nil
}
