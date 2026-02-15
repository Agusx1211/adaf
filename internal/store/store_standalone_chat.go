package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func safeDirName(name string) string {
	return safeNameRe.ReplaceAllString(strings.ToLower(name), "_")
}

func (s *Store) ListStandaloneChatMessages(profileName string) ([]StandaloneChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.localDir("standalonechat", safeDirName(profileName))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var messages []StandaloneChatMessage
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var msg StandaloneChatMessage
		if err := s.readJSON(filepath.Join(dir, e.Name()), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })
	return messages, nil
}

func (s *Store) CreateStandaloneChatMessage(profileName string, msg *StandaloneChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.localDir("standalonechat", safeDirName(profileName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	msg.ID = s.nextID(dir)
	msg.Profile = profileName
	msg.CreatedAt = time.Now().UTC()
	filename := fmt.Sprintf("%d.json", msg.ID)
	return s.writeJSON(filepath.Join(dir, filename), msg)
}

// ReadStandaloneChatLastSession returns the last daemon session ID for a profile-level chat.
func (s *Store) ReadStandaloneChatLastSession(profileName string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p := filepath.Join(s.localDir("standalonechat", safeDirName(profileName)), "last_session_id")
	data, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	var id int
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &id)
	return id
}

// WriteStandaloneChatLastSession stores the last daemon session ID for a profile-level chat.
func (s *Store) WriteStandaloneChatLastSession(profileName string, sessionID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.localDir("standalonechat", safeDirName(profileName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "last_session_id"), []byte(fmt.Sprintf("%d", sessionID)), 0644)
}

func (s *Store) ClearStandaloneChatMessages(profileName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.localDir("standalonechat", safeDirName(profileName))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") || e.Name() == "last_session_id" {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
