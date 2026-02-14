package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Store persistence is organized across domain-specific store_*.go files.
// This file contains core store infrastructure and shared helpers.
const AdafDir = ".adaf"

type Store struct {
	root string // path to .adaf directory
	mu   sync.RWMutex

	signalMu         sync.Mutex
	waitSignals      map[int]chan struct{}
	interruptSignals map[int]chan string
}

var requiredProjectSubdirs = []string{
	"turns",
	"records",
	"plans",
	"docs",
	"issues",
	"spawns",
	"messages",
	"loopruns",
	"stats",
	"stats/profiles",
	"stats/loops",
	"waits",
	"interrupts",
}

func New(projectDir string) (*Store, error) {
	root := filepath.Join(projectDir, AdafDir)
	return &Store{
		root:             root,
		waitSignals:      make(map[int]chan struct{}),
		interruptSignals: make(map[int]chan string),
	}, nil
}

func (s *Store) Init(config ProjectConfig) error {
	if _, err := s.ensureProjectDirs(); err != nil {
		return fmt.Errorf("creating project store directories: %w", err)
	}

	config.Created = time.Now().UTC()
	return s.writeJSON(filepath.Join(s.root, "project.json"), config)
}

func (s *Store) Exists() bool {
	_, err := os.Stat(filepath.Join(s.root, "project.json"))
	return err == nil
}

func (s *Store) Root() string {
	return s.root
}

// Project config

func (s *Store) LoadProject() (*ProjectConfig, error) {
	var config ProjectConfig
	if err := s.readJSON(filepath.Join(s.root, "project.json"), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (s *Store) SaveProject(config *ProjectConfig) error {
	return s.writeJSON(filepath.Join(s.root, "project.json"), config)
}

func (s *Store) writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (s *Store) readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (s *Store) nextID(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}
	maxID := 0
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if id, err := strconv.Atoi(name); err == nil && id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}

func lockFile(path string) (*os.File, error) {
	lf, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, err
	}
	return lf, nil
}

// unlockFile releases the flock and closes the lock file.

func unlockFile(lf *os.File) {
	if lf == nil {
		return
	}
	syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	lf.Close()
}

// writeJSONLocked writes JSON to path while holding an flock.

func (s *Store) writeJSONLocked(path string, v any) error {
	lf, err := lockFile(path)
	if err != nil {
		return fmt.Errorf("lock %s: %w", path, err)
	}
	defer unlockFile(lf)
	return s.writeJSON(path, v)
}

// readJSONLocked reads JSON from path while holding a shared flock.

func (s *Store) readJSONLocked(path string, v any) error {
	lf, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return s.readJSON(path, v) // fallback to unlocked
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_SH); err != nil {
		lf.Close()
		return s.readJSON(path, v) // fallback
	}
	defer unlockFile(lf)
	return s.readJSON(path, v)
}

func (s *Store) EnsureDirs() error {
	_, err := s.Repair()
	return err
}

// Repair recreates missing project store directories.
// It returns a list of created relative directory paths (for reporting).

func (s *Store) Repair() ([]string, error) {
	created, err := s.ensureProjectDirs()
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) ensureProjectDirs() ([]string, error) {
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return nil, err
	}

	created := make([]string, 0, len(requiredProjectSubdirs))
	for _, sub := range requiredProjectSubdirs {
		path := filepath.Join(s.root, sub)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				created = append(created, filepath.Join(AdafDir, sub))
			} else {
				return nil, err
			}
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, err
		}
	}
	return created, nil
}
