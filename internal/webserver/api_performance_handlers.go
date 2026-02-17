package webserver

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/profilescore"
	"github.com/agusx1211/adaf/internal/store"
)

type spawnFeedbackRequest struct {
	Difficulty     *float64 `json:"difficulty"`
	Quality        *float64 `json:"quality"`
	Notes          string   `json:"notes,omitempty"`
	ParentRole     string   `json:"parent_role,omitempty"`
	ParentPosition string   `json:"parent_position,omitempty"`
}

type profilePerformanceDetailResponse struct {
	Summary profilescore.ProfileSummary   `json:"summary"`
	Records []profilescore.FeedbackRecord `json:"records"`
}

func loadPerformanceCatalog() []profilescore.ProfileCatalogEntry {
	cfg, err := config.Load()
	if err != nil || cfg == nil {
		return []profilescore.ProfileCatalogEntry{}
	}
	catalog := make([]profilescore.ProfileCatalogEntry, 0, len(cfg.Profiles))
	for _, prof := range cfg.Profiles {
		name := strings.TrimSpace(prof.Name)
		if name == "" {
			continue
		}
		catalog = append(catalog, profilescore.ProfileCatalogEntry{
			Name: name,
			Cost: config.NormalizeProfileCost(prof.Cost),
		})
	}
	sort.Slice(catalog, func(i, j int) bool {
		return strings.ToLower(catalog[i].Name) < strings.ToLower(catalog[j].Name)
	})
	return catalog
}

func (srv *Server) handleListProfilePerformance(w http.ResponseWriter, r *http.Request) {
	records, err := profilescore.Default().ListFeedback()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile feedback")
		return
	}
	report := profilescore.BuildDashboard(loadPerformanceCatalog(), records)
	writeJSON(w, http.StatusOK, report)
}

func (srv *Server) handleProfilePerformanceByName(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	records, err := profilescore.Default().ListFeedback()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load profile feedback")
		return
	}
	report := profilescore.BuildDashboard(loadPerformanceCatalog(), records)
	var summary profilescore.ProfileSummary
	found := false
	for i := range report.Profiles {
		if strings.EqualFold(report.Profiles[i].Profile, name) {
			summary = report.Profiles[i]
			found = true
			break
		}
	}
	if !found {
		summary = profilescore.ProfileSummary{
			Profile: strings.TrimSpace(name),
		}
		for _, c := range loadPerformanceCatalog() {
			if strings.EqualFold(c.Name, name) {
				summary.Cost = c.Cost
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, profilePerformanceDetailResponse{
		Summary: summary,
		Records: profilescore.FeedbackForProfile(records, name),
	})
}

func handleCreateSpawnFeedbackP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	spawnID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "spawn not found")
		return
	}

	var req spawnFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Difficulty == nil || req.Quality == nil {
		writeError(w, http.StatusBadRequest, "difficulty and quality are required")
		return
	}

	spawnRec, err := s.GetSpawn(spawnID)
	if err != nil {
		if isNotFoundErr(err) {
			writeError(w, http.StatusNotFound, "spawn not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load spawn")
		return
	}
	if !store.IsTerminalSpawnStatus(spawnRec.Status) {
		writeError(w, http.StatusBadRequest, "spawn feedback can only be recorded after the spawn is completed")
		return
	}

	durationSecs := 0
	if !spawnRec.StartedAt.IsZero() && !spawnRec.CompletedAt.IsZero() && spawnRec.CompletedAt.After(spawnRec.StartedAt) {
		durationSecs = int(spawnRec.CompletedAt.Sub(spawnRec.StartedAt).Seconds())
	}

	projectName := ""
	if cfg, err := s.LoadProject(); err == nil && cfg != nil {
		projectName = strings.TrimSpace(cfg.Name)
	}

	rec := profilescore.FeedbackRecord{
		ProjectID:      strings.TrimSpace(s.ProjectID()),
		ProjectName:    projectName,
		SpawnID:        spawnRec.ID,
		ParentTurnID:   spawnRec.ParentTurnID,
		ChildTurnID:    spawnRec.ChildTurnID,
		ParentProfile:  spawnRec.ParentProfile,
		ParentRole:     strings.ToLower(strings.TrimSpace(req.ParentRole)),
		ParentPosition: strings.ToLower(strings.TrimSpace(req.ParentPosition)),
		ChildProfile:   spawnRec.ChildProfile,
		ChildRole:      spawnRec.ChildRole,
		ChildPosition:  spawnRec.ChildPosition,
		ChildStatus:    spawnRec.Status,
		ExitCode:       spawnRec.ExitCode,
		Task:           spawnRec.Task,
		DurationSecs:   durationSecs,
		Difficulty:     *req.Difficulty,
		Quality:        *req.Quality,
		Notes:          strings.TrimSpace(req.Notes),
	}

	created, err := profilescore.Default().UpsertFeedback(rec)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "must be between") || strings.Contains(strings.ToLower(err.Error()), "required") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to save spawn feedback")
		return
	}

	writeJSON(w, http.StatusOK, created)
}
