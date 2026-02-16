package webserver

import (
	"context"
	"net/http"
	"time"

	"github.com/agusx1211/adaf/internal/usage"
)

type usageLimitResponse struct {
	Name           string  `json:"name"`
	UtilizationPct float64 `json:"utilization_pct"`
	RemainingPct   float64 `json:"remaining_pct"`
	ResetsAt       string  `json:"resets_at,omitempty"`
	Level          string  `json:"level"`
}

type usageSnapshotResponse struct {
	Provider  string               `json:"provider"`
	Level     string               `json:"level"`
	Timestamp string               `json:"timestamp"`
	Limits    []usageLimitResponse `json:"limits"`
}

type usageResponse struct {
	Snapshots []usageSnapshotResponse `json:"snapshots"`
	Errors    []string                `json:"errors,omitempty"`
}

func (srv *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	providers := usage.DefaultProviders()

	var resp usageResponse
	for _, p := range providers {
		if !p.HasCredentials() {
			continue
		}

		snapshot, err := p.FetchUsage(ctx)
		if err != nil {
			resp.Errors = append(resp.Errors, err.Error())
			continue
		}

		if len(snapshot.Limits) == 0 {
			continue
		}

		var limits []usageLimitResponse
		for _, l := range snapshot.Limits {
			resetsAt := ""
			if l.ResetsAt != nil {
				resetsAt = l.ResetsAt.Format(time.RFC3339)
			}
			limits = append(limits, usageLimitResponse{
				Name:           l.Name,
				UtilizationPct: l.UtilizationPct,
				RemainingPct:   l.RemainingPct(),
				ResetsAt:       resetsAt,
				Level:          l.Level(70, 90).String(),
			})
		}

		resp.Snapshots = append(resp.Snapshots, usageSnapshotResponse{
			Provider:  snapshot.Provider.DisplayName(),
			Level:     snapshot.OverallLevel.String(),
			Timestamp: snapshot.Timestamp.Format(time.RFC3339),
			Limits:    limits,
		})
	}

	writeJSON(w, http.StatusOK, resp)
}
