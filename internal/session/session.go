package session

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/config"
)

const (
	// Session lifecycle statuses stored in SessionMeta.Status.
	StatusStarting  = "starting"
	StatusRunning   = "running"
	StatusDone      = "done"
	StatusCancelled = "cancelled"
	StatusError     = "error"
	StatusDead      = "dead"
)

// SessionMeta describes a running or completed session daemon.
type SessionMeta struct {
	ID          int       `json:"id"`
	ProfileName string    `json:"profile_name"`
	AgentName   string    `json:"agent_name"`
	LoopName    string    `json:"loop_name,omitempty"`
	LoopSteps   int       `json:"loop_steps,omitempty"`
	ProjectDir  string    `json:"project_dir"`
	ProjectName string    `json:"project_name"`
	PID         int       `json:"pid"`
	Status      string    `json:"status"` // one of StatusStarting/StatusRunning/StatusDone/StatusCancelled/StatusError/StatusDead
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// IsActiveStatus returns whether a session status represents a live/running daemon.
func IsActiveStatus(status string) bool {
	return status == StatusStarting || status == StatusRunning
}

// DaemonConfig holds everything the daemon process needs to reconstruct and
// run the agent loop. Written to disk by the parent before starting the daemon.
type DaemonConfig struct {
	ProjectDir  string `json:"project_dir"`
	ProjectName string `json:"project_name"`
	WorkDir     string `json:"work_dir"`
	PlanID      string `json:"plan_id,omitempty"`

	// Display fields used in session listings and attach headers.
	ProfileName string `json:"profile_name"`
	AgentName   string `json:"agent_name"`

	// Loop execution snapshot.
	Loop      config.LoopDef        `json:"loop"`
	Profiles  []config.Profile      `json:"profiles"`
	Pushover  config.PushoverConfig `json:"pushover,omitempty"`
	MaxCycles int                   `json:"max_cycles,omitempty"` // 0 = unlimited

	// Optional one-off command path overrides keyed by agent name.
	AgentCommandOverrides map[string]string `json:"agent_command_overrides,omitempty"`
}

// Dir returns the global sessions directory (~/.adaf/sessions/), creating it if needed.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".adaf", "sessions")
	os.MkdirAll(dir, 0755)
	return dir
}

// SessionDir returns the directory for a specific session.
func SessionDir(id int) string {
	return filepath.Join(Dir(), fmt.Sprintf("%d", id))
}

// SocketPath returns the Unix socket path for a session.
func SocketPath(id int) string {
	return filepath.Join(SessionDir(id), "sock")
}

// MetaPath returns the metadata JSON path for a session.
func MetaPath(id int) string {
	return filepath.Join(SessionDir(id), "meta.json")
}

// ConfigPath returns the daemon config JSON path for a session.
func ConfigPath(id int) string {
	return filepath.Join(SessionDir(id), "config.json")
}

// EventsPath returns the events JSONL path for a session.
func EventsPath(id int) string {
	return filepath.Join(SessionDir(id), "events.jsonl")
}

// DaemonLogPath returns the daemon stdout/stderr log path for a session.
func DaemonLogPath(id int) string {
	return filepath.Join(SessionDir(id), "daemon.log")
}

// nextID returns the next available session ID.
func nextID() int {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1
	}
	maxID := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if id, err := strconv.Atoi(e.Name()); err == nil && id > maxID {
			maxID = id
		}
	}
	return maxID + 1
}

// CreateSession allocates a new session ID, writes the DaemonConfig and initial
// SessionMeta to disk, and returns the session ID.
func CreateSession(dcfg DaemonConfig) (int, error) {
	const maxAttempts = 256

	var (
		id      int
		created bool
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		id = nextID()
		if err := os.Mkdir(SessionDir(id), 0755); err != nil {
			if os.IsExist(err) {
				continue
			}
			return 0, fmt.Errorf("creating session dir: %w", err)
		}
		created = true
		break
	}
	if !created {
		return 0, fmt.Errorf("allocating session id: too much contention")
	}

	// Write daemon config.
	data, err := json.MarshalIndent(dcfg, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(ConfigPath(id), data, 0644); err != nil {
		return 0, err
	}

	// Write initial metadata.
	meta := SessionMeta{
		ID:          id,
		ProfileName: dcfg.ProfileName,
		AgentName:   dcfg.AgentName,
		LoopName:    dcfg.Loop.Name,
		LoopSteps:   len(dcfg.Loop.Steps),
		ProjectDir:  dcfg.ProjectDir,
		ProjectName: dcfg.ProjectName,
		Status:      StatusStarting,
		StartedAt:   time.Now().UTC(),
	}
	return id, SaveMeta(id, &meta)
}

// SaveMeta writes session metadata to disk.
func SaveMeta(id int, meta *SessionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(MetaPath(id), data, 0644)
}

// LoadMeta reads session metadata from disk.
func LoadMeta(id int) (*SessionMeta, error) {
	data, err := os.ReadFile(MetaPath(id))
	if err != nil {
		return nil, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// LoadConfig reads the daemon config from disk.
func LoadConfig(id int) (*DaemonConfig, error) {
	data, err := os.ReadFile(ConfigPath(id))
	if err != nil {
		return nil, err
	}
	var cfg DaemonConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ListSessions returns all sessions, sorted by ID descending (newest first).
// Stale sessions (where the PID is dead) are marked as StatusDead.
func ListSessions() ([]SessionMeta, error) {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []SessionMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sessionID, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}

		metaPath := filepath.Join(dir, e.Name(), "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta SessionMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if meta.ID == 0 {
			meta.ID = sessionID
		}

		// Check if the daemon is still alive for running sessions.
		if IsActiveStatus(meta.Status) {
			if !isProcessAlive(meta.PID) {
				meta.Status = StatusDead
				meta.EndedAt = time.Now().UTC()
				meta.Error = "daemon process died unexpectedly"
				_ = SaveMeta(meta.ID, &meta)
			}
		}

		sessions = append(sessions, meta)
	}

	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ID > sessions[j].ID })
	return sessions, nil
}

// ListActiveSessions returns only sessions that are currently running.
func ListActiveSessions() ([]SessionMeta, error) {
	all, err := ListSessions()
	if err != nil {
		return nil, err
	}
	var active []SessionMeta
	for _, s := range all {
		if IsActiveStatus(s.Status) {
			active = append(active, s)
		}
	}
	return active, nil
}

// CleanupOld removes session directories older than the given duration
// that are not currently running.
func CleanupOld(maxAge time.Duration) error {
	sessions, err := ListSessions()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)
	for _, s := range sessions {
		if IsActiveStatus(s.Status) {
			continue
		}
		if s.EndedAt.Before(cutoff) || (s.EndedAt.IsZero() && s.StartedAt.Before(cutoff)) {
			os.RemoveAll(SessionDir(s.ID))
		}
	}
	return nil
}

// AbortSessionStartup best-effort cancels and terminates a just-started session.
// It is intended for startup failures where the caller created a session daemon
// but could not attach to it.
func AbortSessionStartup(id int, reason string) {
	_ = sendCancelControl(id)

	meta, err := LoadMeta(id)
	if err != nil || meta == nil {
		return
	}

	if IsActiveStatus(meta.Status) && isProcessAlive(meta.PID) {
		if meta.PID > 0 {
			_ = syscall.Kill(-meta.PID, syscall.SIGTERM)
			_ = syscall.Kill(meta.PID, syscall.SIGTERM)
		}
	}

	if IsActiveStatus(meta.Status) {
		meta.Status = StatusCancelled
	}
	if strings.TrimSpace(reason) != "" {
		meta.Error = reason
	}
	if meta.EndedAt.IsZero() {
		meta.EndedAt = time.Now().UTC()
	}
	_ = SaveMeta(id, meta)
}

func sendCancelControl(id int) error {
	conn, err := net.DialTimeout("unix", SocketPath(id), 500*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = fmt.Fprintf(conn, "%s\n", CtrlCancel)
	return err
}

// IsAgentContext returns true if the current process is running inside an
// adaf agent session (spawned by the orchestrator or launched as an agent).
// Session management commands should refuse to run in this context.
func IsAgentContext() bool {
	return strings.TrimSpace(os.Getenv("ADAF_TURN_ID")) != "" ||
		strings.TrimSpace(os.Getenv("ADAF_SESSION_ID")) != "" ||
		os.Getenv("ADAF_AGENT") == "1"
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check liveness.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// FormatElapsed returns a human-readable elapsed time string.
func FormatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// FormatTimeAgo returns a human-readable "time ago" string.
func FormatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		return t.Format("Jan 2 15:04")
	}
}

// FindOnlyRunningSession returns the single running session if exactly one exists.
// Returns an error if zero or more than one session is running.
func FindOnlyRunningSession() (*SessionMeta, error) {
	active, err := ListActiveSessions()
	if err != nil {
		return nil, err
	}
	if len(active) == 0 {
		return nil, fmt.Errorf("no running sessions")
	}
	if len(active) > 1 {
		var hints []string
		for _, s := range active {
			label := fmt.Sprintf("#%d", s.ID)
			if s.LoopName != "" {
				label += " (" + s.LoopName + ")"
			} else if s.ProfileName != "" {
				label += " (" + s.ProfileName + ")"
			}
			hints = append(hints, label)
		}
		return nil, fmt.Errorf("multiple running sessions: %s — specify a loop name or session ID", strings.Join(hints, ", "))
	}
	return &active[0], nil
}

// FindRunningByLoopName finds a running session by its loop name (case-insensitive).
// Returns an error if no running session matches or if multiple match.
func FindRunningByLoopName(name string) (*SessionMeta, error) {
	active, err := ListActiveSessions()
	if err != nil {
		return nil, err
	}

	requested := strings.TrimSpace(name)
	lookup := strings.ToLower(requested)
	var matches []SessionMeta
	for _, s := range active {
		if strings.ToLower(s.LoopName) == lookup {
			matches = append(matches, s)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no running session for loop %q", requested)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple running sessions for loop %q, specify the session ID", requested)
	}
	return &matches[0], nil
}

// FindSessionByPartial finds a session by numeric ID or partial profile name match.
func FindSessionByPartial(query string) (*SessionMeta, error) {
	requested := strings.TrimSpace(query)

	// Try numeric ID first.
	if id, err := strconv.Atoi(requested); err == nil {
		meta, err := LoadMeta(id)
		if err == nil {
			return meta, nil
		}
	}

	// Try profile name match.
	sessions, err := ListSessions()
	if err != nil {
		return nil, err
	}

	lookup := strings.ToLower(requested)
	var matches []SessionMeta
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.ProfileName), lookup) {
			matches = append(matches, s)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no session matching %q", requested)
	}
	if len(matches) > 1 {
		// Prefer running sessions.
		for _, m := range matches {
			if m.Status == StatusRunning {
				return &m, nil
			}
		}
		return nil, fmt.Errorf("multiple sessions match %q, specify the numeric ID", requested)
	}
	return &matches[0], nil
}
