package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const AdafDir = ".adaf"

type Store struct {
	root string // path to .adaf directory
	mu   sync.RWMutex
}

func New(projectDir string) (*Store, error) {
	root := filepath.Join(projectDir, AdafDir)
	return &Store{root: root}, nil
}

func (s *Store) Init(config ProjectConfig) error {
	dirs := []string{
		s.root,
		filepath.Join(s.root, "logs"),
		filepath.Join(s.root, "records"),
		filepath.Join(s.root, "docs"),
		filepath.Join(s.root, "issues"),
		filepath.Join(s.root, "decisions"),
		filepath.Join(s.root, "spawns"),
		filepath.Join(s.root, "notes"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
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

// Plan

func (s *Store) LoadPlan() (*Plan, error) {
	path := filepath.Join(s.root, "plan.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &Plan{Updated: time.Now().UTC()}, nil
	}
	var plan Plan
	if err := s.readJSON(path, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *Store) SavePlan(plan *Plan) error {
	plan.Updated = time.Now().UTC()
	return s.writeJSON(filepath.Join(s.root, "plan.json"), plan)
}

// Issues

func (s *Store) ListIssues() ([]Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "issues")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var issues []Issue
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var issue Issue
		if err := s.readJSON(filepath.Join(dir, e.Name()), &issue); err != nil {
			continue
		}
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues, nil
}

func (s *Store) CreateIssue(issue *Issue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	issue.ID = s.nextID(filepath.Join(s.root, "issues"))
	issue.Created = time.Now().UTC()
	issue.Updated = issue.Created
	if issue.Status == "" {
		issue.Status = "open"
	}
	return s.writeJSON(filepath.Join(s.root, "issues", fmt.Sprintf("%d.json", issue.ID)), issue)
}

func (s *Store) GetIssue(id int) (*Issue, error) {
	var issue Issue
	if err := s.readJSON(filepath.Join(s.root, "issues", fmt.Sprintf("%d.json", id)), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (s *Store) UpdateIssue(issue *Issue) error {
	issue.Updated = time.Now().UTC()
	return s.writeJSON(filepath.Join(s.root, "issues", fmt.Sprintf("%d.json", issue.ID)), issue)
}

// Session logs

func (s *Store) ListLogs() ([]SessionLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var logs []SessionLog
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var log SessionLog
		if err := s.readJSON(filepath.Join(dir, e.Name()), &log); err != nil {
			continue
		}
		logs = append(logs, log)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ID < logs[j].ID })
	return logs, nil
}

func (s *Store) CreateLog(log *SessionLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.ID = s.nextID(filepath.Join(s.root, "logs"))
	log.Date = time.Now().UTC()
	return s.writeJSON(filepath.Join(s.root, "logs", fmt.Sprintf("%d.json", log.ID)), log)
}

func (s *Store) GetLog(id int) (*SessionLog, error) {
	var log SessionLog
	if err := s.readJSON(filepath.Join(s.root, "logs", fmt.Sprintf("%d.json", id)), &log); err != nil {
		return nil, err
	}
	return &log, nil
}

func (s *Store) UpdateLog(log *SessionLog) error {
	return s.writeJSON(filepath.Join(s.root, "logs", fmt.Sprintf("%d.json", log.ID)), log)
}

func (s *Store) LatestLog() (*SessionLog, error) {
	logs, err := s.ListLogs()
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[len(logs)-1], nil
}

// Docs

func (s *Store) ListDocs() ([]Doc, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "docs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var docs []Doc
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var doc Doc
		if err := s.readJSON(filepath.Join(dir, e.Name()), &doc); err != nil {
			continue
		}
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	return docs, nil
}

func (s *Store) CreateDoc(doc *Doc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if doc.ID == "" {
		doc.ID = fmt.Sprintf("%d", s.nextID(filepath.Join(s.root, "docs")))
	}
	doc.Created = time.Now().UTC()
	doc.Updated = doc.Created
	return s.writeJSON(filepath.Join(s.root, "docs", doc.ID+".json"), doc)
}

func (s *Store) GetDoc(id string) (*Doc, error) {
	var doc Doc
	if err := s.readJSON(filepath.Join(s.root, "docs", id+".json"), &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (s *Store) UpdateDoc(doc *Doc) error {
	doc.Updated = time.Now().UTC()
	return s.writeJSON(filepath.Join(s.root, "docs", doc.ID+".json"), doc)
}

// Decisions

func (s *Store) ListDecisions() ([]Decision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "decisions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var decisions []Decision
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var dec Decision
		if err := s.readJSON(filepath.Join(dir, e.Name()), &dec); err != nil {
			continue
		}
		decisions = append(decisions, dec)
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].ID < decisions[j].ID })
	return decisions, nil
}

func (s *Store) CreateDecision(dec *Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dec.ID = s.nextID(filepath.Join(s.root, "decisions"))
	dec.Date = time.Now().UTC()
	return s.writeJSON(filepath.Join(s.root, "decisions", fmt.Sprintf("%d.json", dec.ID)), dec)
}

func (s *Store) GetDecision(id int) (*Decision, error) {
	var dec Decision
	if err := s.readJSON(filepath.Join(s.root, "decisions", fmt.Sprintf("%d.json", id)), &dec); err != nil {
		return nil, err
	}
	return &dec, nil
}

// Records (formerly "recordings")

func (s *Store) SaveRecording(rec *SessionRecording) error {
	dir := filepath.Join(s.root, "records", fmt.Sprintf("%d", rec.SessionID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return s.writeJSON(filepath.Join(dir, "recording.json"), rec)
}

func (s *Store) AppendRecordingEvent(sessionID int, event RecordingEvent) error {
	dir := filepath.Join(s.root, "records", fmt.Sprintf("%d", sessionID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

func (s *Store) LoadRecording(sessionID int) (*SessionRecording, error) {
	// Try records/ first, fall back to recordings/ for backward compat.
	var rec SessionRecording
	path := filepath.Join(s.root, "records", fmt.Sprintf("%d", sessionID), "recording.json")
	if err := s.readJSON(path, &rec); err != nil {
		// Try legacy path.
		legacyPath := filepath.Join(s.root, "recordings", fmt.Sprintf("%d", sessionID), "recording.json")
		if err2 := s.readJSON(legacyPath, &rec); err2 != nil {
			return nil, err // return original error
		}
	}
	return &rec, nil
}

// RecordsDirs returns paths to scan for session recording directories,
// including the legacy "recordings/" path for backward compatibility.
func (s *Store) RecordsDirs() []string {
	dirs := []string{filepath.Join(s.root, "records")}
	legacyDir := filepath.Join(s.root, "recordings")
	if info, err := os.Stat(legacyDir); err == nil && info.IsDir() {
		dirs = append(dirs, legacyDir)
	}
	return dirs
}

// Helpers

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

// --- File-level locking helpers for multi-process safety ---

// lockFile acquires an exclusive flock on path+".lock". Returns the lock file
// which must be closed (via unlockFile) after the operation completes.
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

// --- Spawns ---

// ListSpawns returns all spawn records.
func (s *Store) ListSpawns() ([]SpawnRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "spawns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var records []SpawnRecord
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var rec SpawnRecord
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &rec); err != nil {
			continue
		}
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

// CreateSpawn persists a new spawn record with an auto-assigned ID.
func (s *Store) CreateSpawn(rec *SpawnRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.root, "spawns")
	os.MkdirAll(dir, 0755)
	rec.ID = s.nextID(dir)
	rec.StartedAt = time.Now().UTC()
	if rec.Status == "" {
		rec.Status = "queued"
	}
	return s.writeJSONLocked(filepath.Join(dir, fmt.Sprintf("%d.json", rec.ID)), rec)
}

// GetSpawn loads a single spawn record by ID.
func (s *Store) GetSpawn(id int) (*SpawnRecord, error) {
	var rec SpawnRecord
	if err := s.readJSONLocked(filepath.Join(s.root, "spawns", fmt.Sprintf("%d.json", id)), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateSpawn persists changes to a spawn record.
func (s *Store) UpdateSpawn(rec *SpawnRecord) error {
	return s.writeJSONLocked(filepath.Join(s.root, "spawns", fmt.Sprintf("%d.json", rec.ID)), rec)
}

// SpawnsByParent returns spawn records created by a given parent session.
func (s *Store) SpawnsByParent(parentSessionID int) ([]SpawnRecord, error) {
	all, err := s.ListSpawns()
	if err != nil {
		return nil, err
	}
	var filtered []SpawnRecord
	for _, r := range all {
		if r.ParentSessionID == parentSessionID {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// --- Supervisor Notes ---

// ListNotes returns all supervisor notes.
func (s *Store) ListNotes() ([]SupervisorNote, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "notes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var notes []SupervisorNote
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var note SupervisorNote
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &note); err != nil {
			continue
		}
		notes = append(notes, note)
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].ID < notes[j].ID })
	return notes, nil
}

// CreateNote persists a new supervisor note with an auto-assigned ID.
func (s *Store) CreateNote(note *SupervisorNote) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.root, "notes")
	os.MkdirAll(dir, 0755)
	note.ID = s.nextID(dir)
	note.CreatedAt = time.Now().UTC()
	return s.writeJSONLocked(filepath.Join(dir, fmt.Sprintf("%d.json", note.ID)), note)
}

// NotesBySession returns notes targeting a given session.
func (s *Store) NotesBySession(sessionID int) ([]SupervisorNote, error) {
	all, err := s.ListNotes()
	if err != nil {
		return nil, err
	}
	var filtered []SupervisorNote
	for _, n := range all {
		if n.SessionID == sessionID {
			filtered = append(filtered, n)
		}
	}
	return filtered, nil
}

// --- Spawn Messages ---

// messagesDir returns the directory for messages of a given spawn.
func (s *Store) messagesDir(spawnID int) string {
	return filepath.Join(s.root, "messages", fmt.Sprintf("%d", spawnID))
}

// CreateMessage persists a new message with an auto-assigned ID.
func (s *Store) CreateMessage(msg *SpawnMessage) error {
	dir := s.messagesDir(msg.SpawnID)
	os.MkdirAll(dir, 0755)

	s.mu.Lock()
	msg.ID = s.nextID(dir)
	s.mu.Unlock()

	msg.CreatedAt = time.Now().UTC()
	return s.writeJSONLocked(filepath.Join(dir, fmt.Sprintf("%d.json", msg.ID)), msg)
}

// ListMessages returns all messages for a spawn, sorted by ID.
func (s *Store) ListMessages(spawnID int) ([]SpawnMessage, error) {
	dir := s.messagesDir(spawnID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var msgs []SpawnMessage
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var msg SpawnMessage
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })
	return msgs, nil
}

// GetMessage loads a single message by spawn and message ID.
func (s *Store) GetMessage(spawnID, msgID int) (*SpawnMessage, error) {
	var msg SpawnMessage
	if err := s.readJSONLocked(filepath.Join(s.messagesDir(spawnID), fmt.Sprintf("%d.json", msgID)), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// MarkMessageRead sets the ReadAt timestamp on a message.
func (s *Store) MarkMessageRead(spawnID, msgID int) error {
	msg, err := s.GetMessage(spawnID, msgID)
	if err != nil {
		return err
	}
	if msg.ReadAt.IsZero() {
		msg.ReadAt = time.Now().UTC()
		return s.writeJSONLocked(filepath.Join(s.messagesDir(spawnID), fmt.Sprintf("%d.json", msgID)), msg)
	}
	return nil
}

// PendingAsk finds an unanswered ask (type=ask with no reply) for a spawn.
func (s *Store) PendingAsk(spawnID int) (*SpawnMessage, error) {
	msgs, err := s.ListMessages(spawnID)
	if err != nil {
		return nil, err
	}

	// Build set of ask IDs that have been replied to.
	replied := make(map[int]bool)
	for _, m := range msgs {
		if m.Type == "reply" && m.ReplyToID > 0 {
			replied[m.ReplyToID] = true
		}
	}

	// Find unanswered asks.
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Type == "ask" && !replied[msgs[i].ID] {
			return &msgs[i], nil
		}
	}
	return nil, nil
}

// UnreadMessages returns unread messages for a spawn in the given direction.
func (s *Store) UnreadMessages(spawnID int, direction string) ([]SpawnMessage, error) {
	msgs, err := s.ListMessages(spawnID)
	if err != nil {
		return nil, err
	}
	var unread []SpawnMessage
	for _, m := range msgs {
		if m.Direction == direction && m.ReadAt.IsZero() {
			unread = append(unread, m)
		}
	}
	return unread, nil
}

// EnsureDirs creates directories that may be missing from older projects.
func (s *Store) EnsureDirs() error {
	for _, sub := range []string{"spawns", "notes", "messages", "loopruns", "stats", "stats/profiles", "stats/loops"} {
		if err := os.MkdirAll(filepath.Join(s.root, sub), 0755); err != nil {
			return err
		}
	}
	return nil
}

// --- Loop Runs ---

// loopRunPath returns the path to a loop run JSON file.
func (s *Store) loopRunPath(id int) string {
	return filepath.Join(s.root, "loopruns", fmt.Sprintf("%d.json", id))
}

// loopRunDir returns the directory for a loop run's associated data.
func (s *Store) loopRunDir(id int) string {
	return filepath.Join(s.root, "loopruns", fmt.Sprintf("%d", id))
}

// CreateLoopRun persists a new loop run with an auto-assigned ID.
func (s *Store) CreateLoopRun(run *LoopRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.root, "loopruns")
	os.MkdirAll(dir, 0755)
	run.ID = s.nextID(dir)
	run.StartedAt = time.Now().UTC()
	if run.Status == "" {
		run.Status = "running"
	}
	if run.StepLastSeenMsg == nil {
		run.StepLastSeenMsg = make(map[int]int)
	}
	return s.writeJSONLocked(s.loopRunPath(run.ID), run)
}

// GetLoopRun loads a single loop run by ID.
func (s *Store) GetLoopRun(id int) (*LoopRun, error) {
	var run LoopRun
	if err := s.readJSONLocked(s.loopRunPath(id), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// UpdateLoopRun persists changes to a loop run.
func (s *Store) UpdateLoopRun(run *LoopRun) error {
	return s.writeJSONLocked(s.loopRunPath(run.ID), run)
}

// ActiveLoopRun finds the currently running loop run, if any.
func (s *Store) ActiveLoopRun() (*LoopRun, error) {
	dir := filepath.Join(s.root, "loopruns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var run LoopRun
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &run); err != nil {
			continue
		}
		if run.Status == "running" {
			return &run, nil
		}
	}
	return nil, nil
}

// --- Loop Messages ---

// loopMessagesDir returns the directory for a loop run's messages.
func (s *Store) loopMessagesDir(runID int) string {
	return filepath.Join(s.loopRunDir(runID), "messages")
}

// CreateLoopMessage persists a new loop message with an auto-assigned ID.
func (s *Store) CreateLoopMessage(msg *LoopMessage) error {
	dir := s.loopMessagesDir(msg.RunID)
	os.MkdirAll(dir, 0755)

	s.mu.Lock()
	msg.ID = s.nextID(dir)
	s.mu.Unlock()

	msg.CreatedAt = time.Now().UTC()
	return s.writeJSONLocked(filepath.Join(dir, fmt.Sprintf("%d.json", msg.ID)), msg)
}

// ListLoopMessages returns all messages for a loop run, sorted by ID.
func (s *Store) ListLoopMessages(runID int) ([]LoopMessage, error) {
	dir := s.loopMessagesDir(runID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var msgs []LoopMessage
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var msg LoopMessage
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &msg); err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })
	return msgs, nil
}

// --- Loop Stop Signal ---

// SignalLoopStop creates a stop signal file for a loop run.
func (s *Store) SignalLoopStop(runID int) error {
	dir := s.loopRunDir(runID)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(filepath.Join(dir, "stop"), []byte("stop"), 0644)
}

// IsLoopStopped checks if a stop signal exists for a loop run.
func (s *Store) IsLoopStopped(runID int) bool {
	_, err := os.Stat(filepath.Join(s.loopRunDir(runID), "stop"))
	return err == nil
}
