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

// GetByID returns the project entry for a given ID.
func (r *ProjectRegistry) GetByID(id string) (*ProjectEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.projects[id]
	if !ok {
		return nil, false
	}
	cp := *entry
	return &cp, true
}

// TryAutoRegister attempts to register a project by reconstructing its filesystem
// path from rootDir + projectID. It checks if the project exists on disk before
// registering. Returns the store and true on success, nil and false on failure.
func (r *ProjectRegistry) TryAutoRegister(rootDir, projectID string) (*store.Store, bool) {
	projectDir := filepath.Join(rootDir, filepath.FromSlash(projectID))
	projectFile := store.ProjectMarkerPath(projectDir)
	if _, err := os.Stat(projectFile); err != nil {
		return nil, false
	}
	if err := r.Register(projectID, projectDir); err != nil {
		return nil, false
	}
	s, ok := r.Get(projectID)
	return s, ok
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
