package webserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func (srv *Server) isWithinAllowedRoot(path string) bool {
	if srv.allowedRoot == "" {
		return true
	}
	rel, err := filepath.Rel(srv.allowedRoot, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

func (srv *Server) resolveBrowsePath(raw string) (string, error) {
	var absPath string
	if raw == "" {
		absPath = filepath.Clean(srv.allowedRoot)
	} else if filepath.IsAbs(raw) {
		absPath = filepath.Clean(raw)
	} else {
		absPath = filepath.Clean(filepath.Join(srv.allowedRoot, raw))
	}
	if !srv.isWithinAllowedRoot(absPath) {
		return "", os.ErrPermission
	}
	return absPath, nil
}

type fsBrowseEntry struct {
	Name      string `json:"name"`
	IsDir     bool   `json:"is_dir"`
	IsProject bool   `json:"is_project"`
	FullPath  string `json:"full_path,omitempty"`
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
		if os.IsPermission(err) {
			writeError(w, http.StatusForbidden, "access denied")
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
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
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		fe := fsBrowseEntry{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
		}

		if entry.IsDir() {
			entryPath := filepath.Join(absPath, entry.Name())
			projectFile := store.ProjectMarkerPath(entryPath)
			if _, serr := os.Stat(projectFile); serr == nil {
				fe.IsProject = true
			}
		}

		result = append(result, fe)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDir != result[j].IsDir {
			return result[i].IsDir
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	parent := ""
	if parentPath := filepath.Dir(absPath); parentPath != absPath && srv.isWithinAllowedRoot(parentPath) {
		parent = parentPath
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

func (srv *Server) handleFSSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]fsBrowseEntry{})
		return
	}

	// If query looks like an absolute path, try to browse it directly
	if filepath.IsAbs(query) {
		absPath, err := srv.resolveBrowsePath(query)
		if err == nil {
			info, err := os.Stat(absPath)
			if err == nil && info.IsDir() {
				// Return the directory itself plus its entries
				dirEntries, _ := os.ReadDir(absPath)
				result := make([]fsBrowseEntry, 0, len(dirEntries)+1)
				// Add the path itself
				projectFile := store.ProjectMarkerPath(absPath)
				isProj := false
				if _, serr := os.Stat(projectFile); serr == nil {
					isProj = true
				}
				result = append(result, fsBrowseEntry{
					Name:      filepath.Base(absPath),
					IsDir:     true,
					IsProject: isProj,
					FullPath:  absPath,
				})
				// Add child directories
				for _, entry := range dirEntries {
					if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
						continue
					}
					entryPath := filepath.Join(absPath, entry.Name())
					fe := fsBrowseEntry{
						Name:     entry.Name(),
						IsDir:    true,
						FullPath: entryPath,
					}
					pf := store.ProjectMarkerPath(entryPath)
					if _, serr := os.Stat(pf); serr == nil {
						fe.IsProject = true
					}
					result = append(result, fe)
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(result)
				return
			}
		}
	}

	// Search through recent projects + scan allowed root for matching directories
	queryLower := strings.ToLower(query)

	// Collect results from recent projects
	cfg, _ := config.Load()
	var results []fsBrowseEntry
	seen := map[string]bool{}

	if cfg != nil {
		for _, rp := range cfg.RecentProjects {
			nameLower := strings.ToLower(rp.Name)
			pathLower := strings.ToLower(rp.Path)
			if strings.Contains(nameLower, queryLower) || strings.Contains(pathLower, queryLower) {
				if !seen[rp.Path] {
					seen[rp.Path] = true
					results = append(results, fsBrowseEntry{
						Name:      rp.Name,
						IsDir:     true,
						IsProject: true,
						FullPath:  rp.Path,
					})
				}
			}
		}
	}

	// Scan first level of allowed root for matching directories
	if srv.allowedRoot != "" {
		dirEntries, err := os.ReadDir(srv.allowedRoot)
		if err == nil {
			for _, entry := range dirEntries {
				if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
					continue
				}
				if !strings.Contains(strings.ToLower(entry.Name()), queryLower) {
					continue
				}
				entryPath := filepath.Join(srv.allowedRoot, entry.Name())
				if seen[entryPath] {
					continue
				}
				seen[entryPath] = true
				fe := fsBrowseEntry{
					Name:     entry.Name(),
					IsDir:    true,
					FullPath: entryPath,
				}
				pf := store.ProjectMarkerPath(entryPath)
				if _, serr := os.Stat(pf); serr == nil {
					fe.IsProject = true
				}
				results = append(results, fe)
			}
		}
	}

	// Cap results
	if len(results) > 25 {
		results = results[:25]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (srv *Server) handleRecentProjects(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg.RecentProjects)
}

func (srv *Server) handleRemoveRecentProject(w http.ResponseWriter, r *http.Request) {
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

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}

	cfg.RemoveRecentProject(req.Path)

	if err := config.Save(cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
	projectFile := store.ProjectMarkerPath(absPath)
	if _, err := os.Stat(projectFile); err != nil {
		writeError(w, http.StatusBadRequest, "not an adaf project (no .adaf.json)")
		return
	}

	// Register in the project registry
	id, err := srv.registry.RegisterByPath(srv.rootDir, absPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register project: "+err.Error())
		return
	}

	go srv.persistRecentProject(id)

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
