package store

import (
	"encoding/json"
	"errors"
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
const (
	// AdafDir is the legacy in-repo data directory name retained for
	// compatibility with existing messages/tests.
	AdafDir = ".adaf"
	// ProjectMarkerFile is the in-repo marker that links a repo directory to
	// a global store under ~/.adaf/projects/<id>.
	ProjectMarkerFile = ".adaf.json"
)

type Store struct {
	projectDir string // path to repo/project directory containing .adaf.json
	projectID  string // value from .adaf.json ("id")
	root       string // path to global store directory (~/.adaf/projects/<id>)
	mu         sync.RWMutex

	signalMu         sync.Mutex
	waitSignals      map[int]chan struct{}
	interruptSignals map[int]chan string
}

var operationalProjectSubdirs = []string{
	"turns",
	"records",
	"spawns",
	"messages",
	"loopruns",
	"stats",
}

var requiredProjectSubdirs = []string{
	"local",
	"local/turns",
	"local/records",
	"plans",
	"docs",
	"issues",
	"local/spawns",
	"local/messages",
	"local/loopruns",
	"local/stats",
	"local/stats/profiles",
	"local/stats/loops",
	"waits",
	"interrupts",
}

func New(projectDir string) (*Store, error) {
	projectDir = cleanPath(projectDir)
	s := &Store{
		projectDir:       projectDir,
		waitSignals:      make(map[int]chan struct{}),
		interruptSignals: make(map[int]chan string),
	}

	projectID, err := ReadProjectID(projectDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading project marker: %w", err)
		}
		return s, nil
	}

	s.projectID = projectID
	s.root = ProjectStoreDirForID(projectID)
	if err := s.migrateToLocalScope(); err != nil {
		return nil, fmt.Errorf("migrating project store layout: %w", err)
	}
	return s, nil
}

func (s *Store) Init(config ProjectConfig) error {
	if strings.TrimSpace(s.projectID) == "" {
		projectID, err := GenerateProjectID(s.projectDir)
		if err != nil {
			return fmt.Errorf("generating project id: %w", err)
		}
		if err := writeProjectMarker(s.projectDir, projectID); err != nil {
			return fmt.Errorf("writing project marker: %w", err)
		}
		s.projectID = projectID
		s.root = ProjectStoreDirForID(projectID)
	}

	if _, err := s.ensureProjectDirs(); err != nil {
		return fmt.Errorf("creating project store directories: %w", err)
	}
	if err := s.ensureStoreGitRepo(); err != nil {
		return fmt.Errorf("initializing project store git repository: %w", err)
	}

	if strings.TrimSpace(config.RepoPath) == "" {
		config.RepoPath = s.projectDir
	}
	config.Created = time.Now().UTC()
	if err := s.writeJSON(filepath.Join(s.root, "project.json"), config); err != nil {
		return err
	}

	// Auto-commit the project initialization
	s.AutoCommit([]string{"project.json"}, "adaf: initialize project")
	return nil
}

func (s *Store) Exists() bool {
	if strings.TrimSpace(s.root) == "" || strings.TrimSpace(s.projectID) == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.root, "project.json"))
	return err == nil
}

func (s *Store) Root() string {
	return s.root
}

func (s *Store) ProjectDir() string {
	return s.projectDir
}

func (s *Store) ProjectID() string {
	return s.projectID
}

func (s *Store) localDir(parts ...string) string {
	all := make([]string, 0, len(parts)+2)
	all = append(all, s.root, "local")
	all = append(all, parts...)
	return filepath.Join(all...)
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
	if err := s.ensureStoreGitRepo(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) ensureProjectDirs() ([]string, error) {
	if strings.TrimSpace(s.root) == "" {
		return nil, fmt.Errorf("project is not initialized (missing %s)", ProjectMarkerFile)
	}
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return nil, err
	}
	if err := s.migrateToLocalScope(); err != nil {
		return nil, err
	}

	created := make([]string, 0, len(requiredProjectSubdirs))
	for _, sub := range requiredProjectSubdirs {
		path := filepath.Join(s.root, sub)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				created = append(created, path)
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

func (s *Store) migrateToLocalScope() error {
	if _, err := os.Stat(s.root); err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return err
	}

	localRoot := s.localDir()
	if err := os.MkdirAll(localRoot, 0755); err != nil {
		return err
	}

	for _, dir := range operationalProjectSubdirs {
		oldPath := filepath.Join(s.root, dir)
		newPath := filepath.Join(localRoot, dir)

		if _, err := os.Stat(oldPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if _, err := os.Stat(newPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := os.Rename(oldPath, newPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
	}

	return nil
}
