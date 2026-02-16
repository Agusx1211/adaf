package webserver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/agusx1211/adaf/internal/store"
)

// ProjectEntry holds a registered project's store and metadata.
type ProjectEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDefault bool   `json:"is_default"`
	store     *store.Store
}

// ProjectRegistry manages multiple project stores for the web server.
type ProjectRegistry struct {
	mu        sync.RWMutex
	projects  map[string]*ProjectEntry
	defaultID string
}

// NewProjectRegistry creates an empty registry.
func NewProjectRegistry() *ProjectRegistry {
	return &ProjectRegistry{
		projects: make(map[string]*ProjectEntry),
	}
}

// Register adds a project store to the registry. The first registered project
// becomes the default unless overridden. If the ID already exists with the
// same path, it returns nil (idempotent).
func (r *ProjectRegistry) Register(id, projectDir string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, exists := r.projects[id]; exists {
		if filepath.Clean(existing.Path) == filepath.Clean(projectDir) {
			return nil // already registered with same path
		}
		return fmt.Errorf("project %q already registered", id)
	}

	s, err := store.New(projectDir)
	if err != nil {
		return fmt.Errorf("opening store for %q: %w", id, err)
	}
	if !s.Exists() {
		return fmt.Errorf("no adaf project found at %s", projectDir)
	}
	if err := s.EnsureDirs(); err != nil {
		return fmt.Errorf("ensuring store dirs for %q: %w", id, err)
	}

	cfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project config for %q: %w", id, err)
	}

	entry := &ProjectEntry{
		ID:    id,
		Name:  cfg.Name,
		Path:  projectDir,
		store: s,
	}

	r.projects[id] = entry
	if r.defaultID == "" {
		r.defaultID = id
		entry.IsDefault = true
	}

	return nil
}

// SetDefault changes the default project ID.
func (r *ProjectRegistry) SetDefault(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.projects[id]; !ok {
		return fmt.Errorf("project %q not found", id)
	}

	// Clear old default
	if old, ok := r.projects[r.defaultID]; ok {
		old.IsDefault = false
	}

	r.defaultID = id
	r.projects[id].IsDefault = true
	return nil
}

// Get returns the store for a given project ID.
func (r *ProjectRegistry) Get(id string) (*store.Store, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.projects[id]
	if !ok {
		return nil, false
	}
	return entry.store, true
}

// Default returns the default project's store.
func (r *ProjectRegistry) Default() (*store.Store, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultID == "" {
		return nil, ""
	}
	entry := r.projects[r.defaultID]
	return entry.store, r.defaultID
}

// List returns all registered project entries sorted by ID.
func (r *ProjectRegistry) List() []ProjectEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]ProjectEntry, 0, len(r.projects))
	for _, e := range r.projects {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}

// Count returns the number of registered projects.
func (r *ProjectRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.projects)
}

// RegisterByPath registers a project by its absolute path, computing the ID
// as the relative path from rootDir.
func (r *ProjectRegistry) RegisterByPath(rootDir, absPath string) (string, error) {
	rel, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		return "", fmt.Errorf("computing relative path: %w", err)
	}
	// Use the relative path as the project ID (e.g. "my-project" or "subdir/my-project").
	id := filepath.ToSlash(rel)
	if err := r.Register(id, absPath); err != nil {
		return "", err
	}
	return id, nil
}

// GetByPath returns the project entry for a given absolute path.
func (r *ProjectRegistry) GetByPath(absPath string) (*ProjectEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cleaned := filepath.Clean(absPath)
	for _, entry := range r.projects {
		if filepath.Clean(entry.Path) == cleaned {
			return entry, true
		}
	}
	return nil, false
}

// ScanDirectory scans a parent directory for subdirectories containing
// .adaf/project.json and registers each one. The directory name is used
// as the project ID.
func (r *ProjectRegistry) ScanDirectory(parentDir string) (int, error) {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return 0, fmt.Errorf("reading directory %s: %w", parentDir, err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(parentDir, entry.Name())
		projectFile := filepath.Join(projectDir, store.AdafDir, "project.json")
		if _, err := os.Stat(projectFile); err != nil {
			continue // not an adaf project
		}

		id := entry.Name()
		if err := r.Register(id, projectDir); err != nil {
			// Skip projects that fail to register (already registered, corrupt, etc.)
			continue
		}
		count++
	}

	return count, nil
}
