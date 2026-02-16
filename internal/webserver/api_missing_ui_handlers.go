package webserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

const (
	missingUISampleBodyLimit = 256 * 1024
	missingUISampleTextLimit = 16 * 1024
	missingUISampleFieldMax  = 256
)

var missingUISamplesMu sync.Mutex

type missingUISampleRequest struct {
	Source       string          `json:"source"`
	Reason       string          `json:"reason"`
	Scope        string          `json:"scope,omitempty"`
	SessionID    int             `json:"session_id,omitempty"`
	TurnID       int             `json:"turn_id,omitempty"`
	SpawnID      int             `json:"spawn_id,omitempty"`
	EventType    string          `json:"event_type,omitempty"`
	Agent        string          `json:"agent,omitempty"`
	Model        string          `json:"model,omitempty"`
	Provider     string          `json:"provider,omitempty"`
	FallbackText string          `json:"fallback_text,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type missingUISampleRecord struct {
	Timestamp    time.Time       `json:"timestamp"`
	ProjectID    string          `json:"project_id,omitempty"`
	ProjectName  string          `json:"project_name,omitempty"`
	ProjectPath  string          `json:"project_path,omitempty"`
	Source       string          `json:"source"`
	Reason       string          `json:"reason"`
	Scope        string          `json:"scope,omitempty"`
	SessionID    int             `json:"session_id,omitempty"`
	TurnID       int             `json:"turn_id,omitempty"`
	SpawnID      int             `json:"spawn_id,omitempty"`
	EventType    string          `json:"event_type,omitempty"`
	Agent        string          `json:"agent,omitempty"`
	Model        string          `json:"model,omitempty"`
	Provider     string          `json:"provider,omitempty"`
	FallbackText string          `json:"fallback_text,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	UserAgent    string          `json:"user_agent,omitempty"`
}

func handleReportMissingUISampleP(s *store.Store, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, missingUISampleBodyLimit)
	defer r.Body.Close()

	var req missingUISampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	source := truncateField(req.Source)
	reason := truncateField(req.Reason)
	if source == "" || reason == "" {
		writeError(w, http.StatusBadRequest, "source and reason are required")
		return
	}

	projectName := ""
	if cfg, err := s.LoadProject(); err == nil && cfg != nil {
		projectName = truncateField(cfg.Name)
	}

	agent := truncateField(req.Agent)
	model := truncateField(req.Model)
	provider := truncateField(req.Provider)
	if provider == "" {
		provider = inferProvider(agent, model)
	}

	record := missingUISampleRecord{
		Timestamp:    time.Now().UTC(),
		ProjectID:    truncateField(strings.TrimSpace(r.PathValue("projectID"))),
		ProjectName:  projectName,
		ProjectPath:  projectDir(s),
		Source:       source,
		Reason:       reason,
		Scope:        truncateField(req.Scope),
		SessionID:    maxInt(0, req.SessionID),
		TurnID:       maxInt(0, req.TurnID),
		SpawnID:      maxInt(0, req.SpawnID),
		EventType:    truncateField(req.EventType),
		Agent:        agent,
		Model:        model,
		Provider:     provider,
		FallbackText: truncateText(req.FallbackText, missingUISampleTextLimit),
		Payload:      sanitizePayload(req.Payload),
		UserAgent:    truncateField(r.UserAgent()),
	}

	path, err := appendMissingUISample(record)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist sample")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":   true,
		"path": path,
	})
}

func appendMissingUISample(record missingUISampleRecord) (string, error) {
	dir := filepath.Join(config.Dir(), "missing_UIs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("samples-%s.jsonl", record.Timestamp.Format("2006-01-02"))
	path := filepath.Join(dir, filename)

	line, err := json.Marshal(record)
	if err != nil {
		return "", err
	}

	missingUISamplesMu.Lock()
	defer missingUISamplesMu.Unlock()

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	if _, err := bw.Write(line); err != nil {
		return "", err
	}
	if err := bw.WriteByte('\n'); err != nil {
		return "", err
	}
	if err := bw.Flush(); err != nil {
		return "", err
	}

	return path, nil
}

func sanitizePayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) <= missingUISampleTextLimit {
		cp := append(json.RawMessage(nil), trimmed...)
		return cp
	}

	wrapped, err := json.Marshal(map[string]any{
		"truncated":      true,
		"original_bytes": len(trimmed),
		"preview_json":   truncateText(trimmed, missingUISampleTextLimit),
	})
	if err != nil {
		return nil
	}
	return wrapped
}

func truncateField(value string) string {
	return truncateText(strings.TrimSpace(value), missingUISampleFieldMax)
}

func truncateText(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func maxInt(min, value int) int {
	if value < min {
		return min
	}
	return value
}

func inferProvider(agent, model string) string {
	a := strings.ToLower(strings.TrimSpace(agent))
	m := strings.ToLower(strings.TrimSpace(model))

	if m != "" {
		if slash := strings.Index(m, "/"); slash > 0 {
			switch m[:slash] {
			case "openai", "anthropic", "google", "mistral", "xai", "meta":
				return m[:slash]
			}
		}
		if strings.Contains(m, "claude") {
			return "anthropic"
		}
		if strings.HasPrefix(m, "gpt-") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") {
			return "openai"
		}
		if strings.Contains(m, "gemini") {
			return "google"
		}
		if strings.Contains(m, "mistral") || strings.Contains(m, "devstral") {
			return "mistral"
		}
	}

	switch a {
	case "codex":
		return "openai"
	case "claude":
		return "anthropic"
	case "gemini":
		return "google"
	case "vibe":
		return "mistral"
	}

	return ""
}
