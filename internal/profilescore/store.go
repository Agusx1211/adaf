package profilescore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/hexid"
)

const (
	dataVersion = 1
	fileName    = "profile-feedback.json"
)

// Store persists cross-project profile performance feedback records.
type Store struct {
	path string
}

// Default returns a Store rooted at ~/.adaf/profile-feedback.json.
func Default() *Store {
	return &Store{path: filepath.Join(config.Dir(), fileName)}
}

// NewAtPath creates a Store bound to path.
func NewAtPath(path string) *Store {
	return &Store{path: strings.TrimSpace(path)}
}

// Path returns the storage file path.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// ListFeedback returns all feedback records sorted by CreatedAt ascending.
func (s *Store) ListFeedback() ([]FeedbackRecord, error) {
	ds, err := s.loadShared()
	if err != nil {
		return nil, err
	}
	return append([]FeedbackRecord(nil), ds.Feedback...), nil
}

// UpsertFeedback creates or updates one feedback record.
//
// Existing feedback is matched by (project_id, spawn_id) when both are set.
func (s *Store) UpsertFeedback(rec FeedbackRecord) (*FeedbackRecord, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, fmt.Errorf("profile score store path is empty")
	}
	normalizeRecord(&rec)
	if err := validateRecord(rec); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, fmt.Errorf("creating profile score dir: %w", err)
	}

	lockFile, err := lockPath(s.path+".lock", true)
	if err != nil {
		return nil, fmt.Errorf("locking profile score store: %w", err)
	}
	defer unlock(lockFile)

	ds, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	idx := findRecordIndex(ds.Feedback, rec)
	if idx >= 0 {
		current := ds.Feedback[idx]
		rec.ID = current.ID
		rec.CreatedAt = current.CreatedAt
		rec.UpdatedAt = now
		ds.Feedback[idx] = rec
	} else {
		if rec.ID == "" {
			rec.ID = fmt.Sprintf("%d-%s", now.UnixNano(), hexid.New())
		}
		if rec.CreatedAt.IsZero() {
			rec.CreatedAt = now
		}
		rec.UpdatedAt = now
		ds.Feedback = append(ds.Feedback, rec)
	}
	ds.Updated = now

	if err := s.writeUnlocked(ds); err != nil {
		return nil, err
	}
	return &rec, nil
}

func normalizeRecord(rec *FeedbackRecord) {
	if rec == nil {
		return
	}
	rec.ProjectID = strings.TrimSpace(rec.ProjectID)
	rec.ProjectName = strings.TrimSpace(rec.ProjectName)
	rec.ParentProfile = strings.TrimSpace(rec.ParentProfile)
	rec.ParentRole = strings.ToLower(strings.TrimSpace(rec.ParentRole))
	rec.ParentPosition = strings.ToLower(strings.TrimSpace(rec.ParentPosition))
	rec.ChildProfile = strings.TrimSpace(rec.ChildProfile)
	rec.ChildRole = strings.ToLower(strings.TrimSpace(rec.ChildRole))
	rec.ChildPosition = strings.ToLower(strings.TrimSpace(rec.ChildPosition))
	rec.ChildStatus = strings.ToLower(strings.TrimSpace(rec.ChildStatus))
	rec.Task = strings.TrimSpace(rec.Task)
	rec.Notes = strings.TrimSpace(rec.Notes)
	if rec.DurationSecs < 0 {
		rec.DurationSecs = 0
	}
}

func validateRecord(rec FeedbackRecord) error {
	if strings.TrimSpace(rec.ChildProfile) == "" {
		return fmt.Errorf("child_profile is required")
	}
	if rec.SpawnID <= 0 {
		return fmt.Errorf("spawn_id must be > 0")
	}
	if rec.Difficulty < MinScore || rec.Difficulty > MaxScore {
		return fmt.Errorf("difficulty must be between %.0f and %.0f", MinScore, MaxScore)
	}
	if rec.Quality < MinScore || rec.Quality > MaxScore {
		return fmt.Errorf("quality must be between %.0f and %.0f", MinScore, MaxScore)
	}
	return nil
}

func findRecordIndex(records []FeedbackRecord, rec FeedbackRecord) int {
	if rec.SpawnID <= 0 {
		return -1
	}
	projectID := strings.ToLower(strings.TrimSpace(rec.ProjectID))
	for i := range records {
		if records[i].SpawnID != rec.SpawnID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(records[i].ProjectID)) == projectID {
			return i
		}
	}
	return -1
}

func (s *Store) loadShared() (*dataset, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, fmt.Errorf("profile score store path is empty")
	}
	lockFile, err := lockPath(s.path+".lock", false)
	if err != nil {
		return nil, fmt.Errorf("locking profile score store: %w", err)
	}
	defer unlock(lockFile)
	return s.loadUnlocked()
}

func (s *Store) loadUnlocked() (*dataset, error) {
	_, err := os.Stat(s.path)
	if os.IsNotExist(err) {
		return &dataset{
			Version:  dataVersion,
			Feedback: []FeedbackRecord{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat profile score store: %w", err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("reading profile score store: %w", err)
	}
	var ds dataset
	if err := json.Unmarshal(raw, &ds); err != nil {
		return nil, fmt.Errorf("parsing profile score store: %w", err)
	}
	if ds.Version <= 0 {
		ds.Version = dataVersion
	}
	if ds.Feedback == nil {
		ds.Feedback = []FeedbackRecord{}
	}
	return &ds, nil
}

func (s *Store) writeUnlocked(ds *dataset) error {
	if ds == nil {
		ds = &dataset{}
	}
	ds.Version = dataVersion
	if ds.Feedback == nil {
		ds.Feedback = []FeedbackRecord{}
	}

	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding profile score store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp profile score store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replacing profile score store: %w", err)
	}
	return nil
}

func lockPath(path string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	lockMode := syscall.LOCK_SH
	if exclusive {
		lockMode = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), lockMode); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func unlock(f *os.File) {
	if f == nil {
		return
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}
