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
		filepath.Join(s.root, "recordings"),
		filepath.Join(s.root, "docs"),
		filepath.Join(s.root, "issues"),
		filepath.Join(s.root, "decisions"),
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

// Recordings

func (s *Store) SaveRecording(rec *SessionRecording) error {
	dir := filepath.Join(s.root, "recordings", fmt.Sprintf("%d", rec.SessionID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return s.writeJSON(filepath.Join(dir, "recording.json"), rec)
}

func (s *Store) AppendRecordingEvent(sessionID int, event RecordingEvent) error {
	dir := filepath.Join(s.root, "recordings", fmt.Sprintf("%d", sessionID))
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
	var rec SessionRecording
	if err := s.readJSON(filepath.Join(s.root, "recordings", fmt.Sprintf("%d", sessionID), "recording.json"), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
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
