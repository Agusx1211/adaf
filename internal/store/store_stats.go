package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Profile Stats ---

// profileStatsDir returns the directory for profile stats files.
func (s *Store) profileStatsDir() string {
	return s.localDir("stats", "profiles")
}

// profileStatsPath returns the path for a specific profile's stats file.
func (s *Store) profileStatsPath(name string) string {
	return filepath.Join(s.profileStatsDir(), name+".json")
}

// GetProfileStats loads stats for a profile, returning an empty struct if none exist.
func (s *Store) GetProfileStats(name string) (*ProfileStats, error) {
	path := s.profileStatsPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &ProfileStats{
			ProfileName: name,
			ToolCalls:   make(map[string]int),
			SpawnedBy:   make(map[string]int),
		}, nil
	}
	var stats ProfileStats
	if err := s.readJSONLocked(path, &stats); err != nil {
		return nil, err
	}
	if stats.ToolCalls == nil {
		stats.ToolCalls = make(map[string]int)
	}
	if stats.SpawnedBy == nil {
		stats.SpawnedBy = make(map[string]int)
	}
	return &stats, nil
}

// SaveProfileStats writes profile stats to disk with file locking.
func (s *Store) SaveProfileStats(stats *ProfileStats) error {
	dir := s.profileStatsDir()
	return s.saveStatsFile(dir, s.profileStatsPath(stats.ProfileName), "profile", stats)
}

// ListProfileStats returns stats for all profiles that have stats files.
func (s *Store) ListProfileStats() ([]ProfileStats, error) {
	return listStatsFiles[ProfileStats](s, s.profileStatsDir())
}

// --- Loop Stats ---

// loopStatsDir returns the directory for loop stats files.
func (s *Store) loopStatsDir() string {
	return s.localDir("stats", "loops")
}

// loopStatsPath returns the path for a specific loop's stats file.
func (s *Store) loopStatsPath(name string) string {
	return filepath.Join(s.loopStatsDir(), name+".json")
}

// GetLoopStats loads stats for a loop, returning an empty struct if none exist.
func (s *Store) GetLoopStats(name string) (*LoopStats, error) {
	path := s.loopStatsPath(name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &LoopStats{
			LoopName:  name,
			StepStats: make(map[string]int),
		}, nil
	}
	var stats LoopStats
	if err := s.readJSONLocked(path, &stats); err != nil {
		return nil, err
	}
	if stats.StepStats == nil {
		stats.StepStats = make(map[string]int)
	}
	return &stats, nil
}

// SaveLoopStats writes loop stats to disk with file locking.
func (s *Store) SaveLoopStats(stats *LoopStats) error {
	dir := s.loopStatsDir()
	return s.saveStatsFile(dir, s.loopStatsPath(stats.LoopName), "loop", stats)
}

// ListLoopStats returns stats for all loops that have stats files.
func (s *Store) ListLoopStats() ([]LoopStats, error) {
	return listStatsFiles[LoopStats](s, s.loopStatsDir())
}

func (s *Store) saveStatsFile(dir, path, label string, stats any) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating %s stats dir: %w", label, err)
	}
	return s.writeJSONLocked(path, stats)
}

func listStatsFiles[T any](s *Store, dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []T
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var item T
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &item); err != nil {
			continue
		}
		result = append(result, item)
	}
	return result, nil
}
