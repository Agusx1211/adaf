package webserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
)

// resolveBrowsePath converts the query path to a clean absolute path.
// If the path is empty it defaults to the server's rootDir.
// Both relative (resolved against rootDir) and absolute paths are accepted.
func (srv *Server) resolveBrowsePath(raw string) (string, error) {
	if raw == "" {
		return filepath.Clean(srv.rootDir), nil
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	return filepath.Clean(filepath.Join(srv.rootDir, raw)), nil
}

type fsBrowseEntry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	IsProject bool   `json:"is_project"`
}

type fsBrowseResponse struct {
	Path    string          `json:"path"`
	Parent  string          `json:"parent"`
	Entries []fsBrowseEntry `json:"entries"`
}

func (srv *Server) handleFSBrowse(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")

	absPath, err := srv.resolveBrowsePath(rawPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "path not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to stat path")
		}
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read directory")
		return
	}

	result := make([]fsBrowseEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		// Skip hidden entries
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fe := fsBrowseEntry{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
		}

		// Check if directory is an adaf project
		if entry.IsDir() {
			entryPath := filepath.Join(absPath, entry.Name())
			projectFile := filepath.Join(entryPath, store.AdafDir, "project.json")
			if _, serr := os.Stat(projectFile); serr == nil {
				fe.IsProject = true
			}
		}

		result = append(result, fe)
	}

	sort.Slice(result, func(i, j int) bool {
		// Directories first, then files; within each group alphabetical
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	// Parent is the directory above, unless we're at filesystem root
	parent := filepath.Dir(absPath)
	if parent == absPath {
		parent = "" // at filesystem root
	}

	resp := fsBrowseResponse{
		Path:    absPath,
		Parent:  parent,
		Entries: result,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (srv *Server) handleFSMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	absPath, err := srv.resolveBrowsePath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"path": absPath, "status": "created"})
}

func (srv *Server) handleProjectInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	absPath, err := srv.resolveBrowsePath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Ensure the directory exists
	if err := os.MkdirAll(absPath, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create directory: "+err.Error())
		return
	}

	s, err := store.New(absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create store: "+err.Error())
		return
	}

	if s.Exists() {
		writeError(w, http.StatusConflict, "project already initialized at this path")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(absPath)
	}

	projCfg := store.ProjectConfig{
		Name:        name,
		RepoPath:    absPath,
		AgentConfig: make(map[string]string),
		Metadata:    make(map[string]any),
	}

	if err := s.Init(projCfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize project: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"path":   absPath,
		"name":   name,
		"status": "initialized",
	})
}

func (srv *Server) handleProjectOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Path) == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	absPath, err := srv.resolveBrowsePath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Verify it's an adaf project
	projectFile := filepath.Join(absPath, store.AdafDir, "project.json")
	if _, err := os.Stat(projectFile); err != nil {
		writeError(w, http.StatusBadRequest, "not an adaf project (no .adaf/project.json)")
		return
	}

	// Register in the project registry
	id, err := srv.registry.RegisterByPath(srv.rootDir, absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register project: "+err.Error())
		return
	}

	entry, ok := srv.registry.GetByPath(absPath)
	if !ok {
		writeError(w, http.StatusInternalServerError, "failed to retrieve registered project")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         id,
		"name":       entry.Name,
		"path":       entry.Path,
		"is_default": entry.IsDefault,
	})
}
