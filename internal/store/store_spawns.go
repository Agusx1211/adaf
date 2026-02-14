// store_spawns.go contains spawn and message management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) ListSpawns() ([]SpawnRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.localDir("spawns")
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

	dir := s.localDir("spawns")
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
	if err := s.readJSONLocked(s.localDir("spawns", fmt.Sprintf("%d.json", id)), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpdateSpawn persists changes to a spawn record.

func (s *Store) UpdateSpawn(rec *SpawnRecord) error {
	return s.writeJSONLocked(s.localDir("spawns", fmt.Sprintf("%d.json", rec.ID)), rec)
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
	return s.localDir("messages", fmt.Sprintf("%d", spawnID))
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
