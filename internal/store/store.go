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

	signalMu         sync.Mutex
	waitSignals      map[int]chan struct{}
	interruptSignals map[int]chan string
}

var requiredProjectSubdirs = []string{
	"turns",
	"records",
	"plans",
	"docs",
	"issues",
	"spawns",
	"messages",
	"loopruns",
	"stats",
	"stats/profiles",
	"stats/loops",
	"waits",
	"interrupts",
}

func New(projectDir string) (*Store, error) {
	root := filepath.Join(projectDir, AdafDir)
	return &Store{
		root:             root,
		waitSignals:      make(map[int]chan struct{}),
		interruptSignals: make(map[int]chan string),
	}, nil
}

func (s *Store) Init(config ProjectConfig) error {
	if _, err := s.ensureProjectDirs(); err != nil {
		return fmt.Errorf("creating project store directories: %w", err)
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
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	plan, err := s.ActivePlan()
	if err != nil {
		return nil, err
	}
	if plan != nil {
		return plan, nil
	}

	return &Plan{Status: "active", Updated: time.Now().UTC()}, nil
}

func (s *Store) SavePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}

	project, err := s.LoadProject()
	if err != nil {
		return err
	}

	if plan.ID == "" {
		if project.ActivePlanID != "" {
			plan.ID = project.ActivePlanID
		} else {
			plan.ID = "default"
		}
	}

	existing, err := s.GetPlan(plan.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		if err := s.CreatePlan(plan); err != nil {
			return err
		}
		return s.SetActivePlan(plan.ID)
	}

	if plan.Created.IsZero() {
		plan.Created = existing.Created
	}
	return s.UpdatePlan(plan)
}

func (s *Store) ListPlans() ([]Plan, error) {
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.plansDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var plans []Plan
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var plan Plan
		if err := s.readJSON(filepath.Join(s.plansDir(), e.Name()), &plan); err != nil {
			continue
		}
		if plan.ID == "" {
			plan.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		if plan.Status == "" {
			plan.Status = "active"
		}
		plans = append(plans, plan)
	}

	sort.Slice(plans, func(i, j int) bool { return plans[i].ID < plans[j].ID })
	return plans, nil
}

func (s *Store) GetPlan(id string) (*Plan, error) {
	if id == "" {
		return nil, nil
	}
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	var plan Plan
	if err := s.readJSON(s.planPath(id), &plan); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if plan.ID == "" {
		plan.ID = id
	}
	if plan.Status == "" {
		plan.Status = "active"
	}
	return &plan, nil
}

func (s *Store) CreatePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}
	if plan.ID == "" {
		return fmt.Errorf("plan ID is required")
	}

	path := s.planPath(plan.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("plan %q already exists", plan.ID)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	now := time.Now().UTC()
	if plan.Status == "" {
		plan.Status = "active"
	}
	if plan.Created.IsZero() {
		plan.Created = now
	}
	plan.Updated = now
	return s.writeJSON(path, plan)
}

func (s *Store) UpdatePlan(plan *Plan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}
	if plan.ID == "" {
		return fmt.Errorf("plan ID is required")
	}

	existing, err := s.GetPlan(plan.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("plan %q does not exist", plan.ID)
	}

	if plan.Status == "" {
		plan.Status = existing.Status
		if plan.Status == "" {
			plan.Status = "active"
		}
	}
	if plan.Created.IsZero() {
		plan.Created = existing.Created
	}
	if plan.Created.IsZero() {
		plan.Created = time.Now().UTC()
	}
	plan.Updated = time.Now().UTC()

	return s.writeJSON(s.planPath(plan.ID), plan)
}

func (s *Store) DeletePlan(id string) error {
	if id == "" {
		return fmt.Errorf("plan ID is required")
	}
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}

	plan, err := s.GetPlan(id)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	if plan.Status != "done" && plan.Status != "cancelled" {
		return fmt.Errorf("plan %q status is %q; only done/cancelled can be deleted", id, plan.Status)
	}

	path := s.planPath(id)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	project, err := s.LoadProject()
	if err == nil && project != nil && project.ActivePlanID == id {
		project.ActivePlanID = ""
		if saveErr := s.SaveProject(project); saveErr != nil {
			return saveErr
		}
	}

	return os.Remove(path)
}

func (s *Store) ActivePlan() (*Plan, error) {
	if err := s.ensurePlanStorage(); err != nil {
		return nil, err
	}

	project, err := s.LoadProject()
	if err != nil {
		return nil, err
	}
	if project.ActivePlanID == "" {
		return nil, nil
	}
	return s.GetPlan(project.ActivePlanID)
}

func (s *Store) SetActivePlan(id string) error {
	if err := s.ensurePlanStorage(); err != nil {
		return err
	}

	project, err := s.LoadProject()
	if err != nil {
		return err
	}

	if id != "" {
		plan, err := s.GetPlan(id)
		if err != nil {
			return err
		}
		if plan == nil {
			return fmt.Errorf("plan %q not found", id)
		}
	}

	project.ActivePlanID = id
	return s.SaveProject(project)
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

func (s *Store) ListIssuesForPlan(planID string) ([]Issue, error) {
	if planID == "" {
		return s.ListSharedIssues()
	}
	issues, err := s.ListIssues()
	if err != nil {
		return nil, err
	}
	var filtered []Issue
	for _, issue := range issues {
		if issue.PlanID == "" || issue.PlanID == planID {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
}

func (s *Store) ListSharedIssues() ([]Issue, error) {
	issues, err := s.ListIssues()
	if err != nil {
		return nil, err
	}
	var filtered []Issue
	for _, issue := range issues {
		if issue.PlanID == "" {
			filtered = append(filtered, issue)
		}
	}
	return filtered, nil
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

// Turns

func (s *Store) turnsDir() string {
	return filepath.Join(s.root, "turns")
}

func (s *Store) ListTurns() ([]Turn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.turnsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var turns []Turn
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var turn Turn
		if err := s.readJSON(filepath.Join(dir, e.Name()), &turn); err != nil {
			continue
		}
		turns = append(turns, turn)
	}
	sort.Slice(turns, func(i, j int) bool { return turns[i].ID < turns[j].ID })
	return turns, nil
}

func (s *Store) CreateTurn(turn *Turn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.turnsDir()
	turn.ID = s.nextID(dir)
	turn.Date = time.Now().UTC()
	return s.writeJSON(filepath.Join(dir, fmt.Sprintf("%d.json", turn.ID)), turn)
}

func (s *Store) GetTurn(id int) (*Turn, error) {
	var turn Turn
	if err := s.readJSON(filepath.Join(s.turnsDir(), fmt.Sprintf("%d.json", id)), &turn); err != nil {
		return nil, err
	}
	return &turn, nil
}

func (s *Store) UpdateTurn(turn *Turn) error {
	return s.writeJSON(filepath.Join(s.turnsDir(), fmt.Sprintf("%d.json", turn.ID)), turn)
}

func (s *Store) LatestTurn() (*Turn, error) {
	turns, err := s.ListTurns()
	if err != nil {
		return nil, err
	}
	if len(turns) == 0 {
		return nil, nil
	}
	return &turns[len(turns)-1], nil
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

func (s *Store) ListDocsForPlan(planID string) ([]Doc, error) {
	if planID == "" {
		return s.ListSharedDocs()
	}
	docs, err := s.ListDocs()
	if err != nil {
		return nil, err
	}
	var filtered []Doc
	for _, doc := range docs {
		if doc.PlanID == "" || doc.PlanID == planID {
			filtered = append(filtered, doc)
		}
	}
	return filtered, nil
}

func (s *Store) ListSharedDocs() ([]Doc, error) {
	docs, err := s.ListDocs()
	if err != nil {
		return nil, err
	}
	var filtered []Doc
	for _, doc := range docs {
		if doc.PlanID == "" {
			filtered = append(filtered, doc)
		}
	}
	return filtered, nil
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

// Records

func (s *Store) SaveRecording(rec *TurnRecording) error {
	dir := filepath.Join(s.root, "records", fmt.Sprintf("%d", rec.TurnID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return s.writeJSON(filepath.Join(dir, "recording.json"), rec)
}

func (s *Store) AppendRecordingEvent(turnID int, event RecordingEvent) error {
	dir := filepath.Join(s.root, "records", fmt.Sprintf("%d", turnID))
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

func (s *Store) LoadRecording(turnID int) (*TurnRecording, error) {
	var rec TurnRecording
	path := filepath.Join(s.root, "records", fmt.Sprintf("%d", turnID), "recording.json")
	if err := s.readJSON(path, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// RecordsDirs returns paths to scan for turn recording directories.
func (s *Store) RecordsDirs() []string {
	return []string{filepath.Join(s.root, "records")}
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

func (s *Store) plansDir() string {
	return filepath.Join(s.root, "plans")
}

func (s *Store) planPath(id string) string {
	return filepath.Join(s.plansDir(), id+".json")
}

func (s *Store) ensurePlanStorage() error {
	if err := os.MkdirAll(s.plansDir(), 0755); err != nil {
		return err
	}
	return nil
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
		rec.Status = "running"
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

// SpawnsByParent returns spawn records created by a given parent turn.
func (s *Store) SpawnsByParent(parentTurnID int) ([]SpawnRecord, error) {
	all, err := s.ListSpawns()
	if err != nil {
		return nil, err
	}
	var filtered []SpawnRecord
	for _, r := range all {
		if r.ParentTurnID == parentTurnID {
			filtered = append(filtered, r)
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
	return created, nil
}

func (s *Store) ensureProjectDirs() ([]string, error) {
	if err := os.MkdirAll(s.root, 0755); err != nil {
		return nil, err
	}

	created := make([]string, 0, len(requiredProjectSubdirs))
	for _, sub := range requiredProjectSubdirs {
		path := filepath.Join(s.root, sub)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				created = append(created, filepath.Join(AdafDir, sub))
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := s.stopRunningLoopRunsLocked(dir); err != nil {
		return err
	}

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

	var latest *LoopRun
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var run LoopRun
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &run); err != nil {
			continue
		}
		if run.Status == "running" {
			if latest == nil || run.ID > latest.ID {
				cp := run
				latest = &cp
			}
		}
	}
	return latest, nil
}

// ListLoopRuns returns all loop runs, sorted by ID.
func (s *Store) ListLoopRuns() ([]LoopRun, error) {
	dir := filepath.Join(s.root, "loopruns")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var runs []LoopRun
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var run LoopRun
		if err := s.readJSONLocked(filepath.Join(dir, e.Name()), &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID < runs[j].ID })
	return runs, nil
}

func (s *Store) stopRunningLoopRunsLocked(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	stoppedAt := time.Now().UTC()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, e.Name())
		var run LoopRun
		if err := s.readJSONLocked(path, &run); err != nil {
			continue
		}
		if run.Status != "running" {
			continue
		}

		run.Status = "stopped"
		run.StoppedAt = stoppedAt
		if err := s.writeJSONLocked(path, &run); err != nil {
			return err
		}
	}

	return nil
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

// --- Wait Signal ---

func (s *Store) waitSignalChan(turnID int) chan struct{} {
	s.signalMu.Lock()
	defer s.signalMu.Unlock()
	ch := s.waitSignals[turnID]
	if ch == nil {
		ch = make(chan struct{}, 1)
		s.waitSignals[turnID] = ch
	}
	return ch
}

// WaitSignalChan returns a process-local notification channel that emits when
// SignalWait is called for the given turn ID.
func (s *Store) WaitSignalChan(turnID int) <-chan struct{} {
	return s.waitSignalChan(turnID)
}

func (s *Store) notifyWaitSignal(turnID int) {
	ch := s.waitSignalChan(turnID)
	select {
	case ch <- struct{}{}:
	default:
	}
}

// ReleaseWaitSignal removes the process-local wait notification channel for a turn.
// This is a memory hygiene helper and does not touch on-disk signal files.
func (s *Store) ReleaseWaitSignal(turnID int) {
	s.signalMu.Lock()
	delete(s.waitSignals, turnID)
	s.signalMu.Unlock()
}

// SignalWait creates a wait signal file for a turn.
// This indicates the agent wants to pause and resume when spawns complete.
func (s *Store) SignalWait(turnID int) error {
	dir := filepath.Join(s.root, "waits")
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d", turnID)), []byte("waiting"), 0644); err != nil {
		return err
	}
	s.notifyWaitSignal(turnID)
	return nil
}

// IsWaiting checks if a wait signal exists for a turn.
func (s *Store) IsWaiting(turnID int) bool {
	_, err := os.Stat(filepath.Join(s.root, "waits", fmt.Sprintf("%d", turnID)))
	return err == nil
}

// ClearWait removes the wait signal for a turn.
func (s *Store) ClearWait(turnID int) error {
	err := os.Remove(filepath.Join(s.root, "waits", fmt.Sprintf("%d", turnID)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	s.ReleaseWaitSignal(turnID)
	return nil
}

// --- Interrupt Signal ---

func (s *Store) interruptSignalChan(turnID int) chan string {
	s.signalMu.Lock()
	defer s.signalMu.Unlock()
	ch := s.interruptSignals[turnID]
	if ch == nil {
		ch = make(chan string, 8)
		s.interruptSignals[turnID] = ch
	}
	return ch
}

// InterruptSignalChan returns a process-local notification channel that emits
// interrupt messages for the given spawn/turn ID.
func (s *Store) InterruptSignalChan(turnID int) <-chan string {
	return s.interruptSignalChan(turnID)
}

func (s *Store) notifyInterruptSignal(turnID int, message string) {
	ch := s.interruptSignalChan(turnID)
	select {
	case ch <- message:
	default:
	}
}

// ReleaseInterruptSignal removes the process-local interrupt notification channel
// for a spawn/turn ID. This is a memory hygiene helper and does not touch on-disk
// signal files.
func (s *Store) ReleaseInterruptSignal(turnID int) {
	s.signalMu.Lock()
	delete(s.interruptSignals, turnID)
	s.signalMu.Unlock()
}

// SignalInterrupt creates an interrupt signal file for a turn.
func (s *Store) SignalInterrupt(turnID int, message string) error {
	dir := filepath.Join(s.root, "interrupts")
	os.MkdirAll(dir, 0755)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d", turnID)), []byte(message), 0644); err != nil {
		return err
	}
	s.notifyInterruptSignal(turnID, message)
	return nil
}

// CheckInterrupt checks for and returns an interrupt message for a turn.
// Returns empty string if no interrupt is pending.
func (s *Store) CheckInterrupt(turnID int) string {
	path := filepath.Join(s.root, "interrupts", fmt.Sprintf("%d", turnID))
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// ClearInterrupt removes the interrupt signal for a spawn.
func (s *Store) ClearInterrupt(spawnID int) error {
	return os.Remove(filepath.Join(s.root, "interrupts", fmt.Sprintf("%d", spawnID)))
}
