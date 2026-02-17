// store_turns.go contains turn management methods.
package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrTurnFrozen = errors.New("turn is frozen")

func (s *Store) turnsDir() string {
	return s.localDir("turns")
}

func IsTurnFrozen(turn *Turn) bool {
	if turn == nil {
		return false
	}
	if !turn.FinalizedAt.IsZero() {
		return true
	}
	buildState := strings.TrimSpace(turn.BuildState)
	if buildState == "" || buildState == "waiting_for_spawns" {
		return false
	}
	if buildState == "success" || buildState == "cancelled" || buildState == "error" {
		return true
	}
	return strings.HasPrefix(buildState, "exit_code_")
}

func (s *Store) ListTurns() ([]Turn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.turnsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var turns []Turn
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var turn Turn
		if err := s.readJSON(filepath.Join(dir, e.Name()), &turn); err != nil {
			continue
		}
		turns = append(turns, turn)
	}
	sort.Slice(turns, func(i, j int) bool { return turns[i].ID < turns[j].ID })
	return turns, nil
}

func (s *Store) CreateTurn(turn *Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.turnsDir()
	turn.ID = s.nextID(dir)
	turn.Date = time.Now().UTC()
	return s.writeJSON(filepath.Join(dir, fmt.Sprintf("%d.json", turn.ID)), turn)
}

func (s *Store) GetTurn(id int) (*Turn, error) {
	var turn Turn
	if err := s.readJSON(filepath.Join(s.turnsDir(), fmt.Sprintf("%d.json", id)), &turn); err != nil {
		return nil, err
	}
	return &turn, nil
}

func (s *Store) UpdateTurn(turn *Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.turnsDir(), fmt.Sprintf("%d.json", turn.ID))
	var existing Turn
	if err := s.readJSON(path, &existing); err != nil {
		return err
	}
	if IsTurnFrozen(&existing) {
		if !existing.FinalizedAt.IsZero() {
			return fmt.Errorf("%w: turn #%d finalized at %s", ErrTurnFrozen, existing.ID, existing.FinalizedAt.Format(time.RFC3339))
		}
		return fmt.Errorf("%w: turn #%d has terminal build state %q", ErrTurnFrozen, existing.ID, existing.BuildState)
	}
	return s.writeJSON(path, turn)
}

func (s *Store) LatestTurn() (*Turn, error) {
	turns, err := s.ListTurns()
	if err != nil {
		return nil, err
	}
	if len(turns) == 0 {
		return nil, nil
	}
	return &turns[len(turns)-1], nil
}
