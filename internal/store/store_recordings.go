// store_recordings.go contains recording management methods.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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
