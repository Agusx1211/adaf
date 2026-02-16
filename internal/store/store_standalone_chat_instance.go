package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/hexid"
)

var safeNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func safeDirName(name string) string {
	return safeNameRe.ReplaceAllString(strings.ToLower(name), "_")
}

func (s *Store) instancesDir() string {
	return s.localDir("standalonechat", "instances")
}

func (s *Store) instanceDir(id string) string {
	return filepath.Join(s.instancesDir(), safeDirName(id))
}

func (s *Store) instanceMessagesDir(id string) string {
	return filepath.Join(s.instanceDir(id), "messages")
}

// ListChatInstances returns all chat instances sorted by most-recently-updated first.
func (s *Store) ListChatInstances() ([]StandaloneChatInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.instancesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var instances []StandaloneChatInstance
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, e.Name(), "meta.json")
		var inst StandaloneChatInstance
		if err := s.readJSON(metaPath, &inst); err != nil {
			continue
		}
		instances = append(instances, inst)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].UpdatedAt.After(instances[j].UpdatedAt)
	})
	return instances, nil
}

// GetChatInstance loads a single chat instance by ID.
func (s *Store) GetChatInstance(id string) (*StandaloneChatInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metaPath := filepath.Join(s.instanceDir(id), "meta.json")
	var inst StandaloneChatInstance
	if err := s.readJSON(metaPath, &inst); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return &inst, nil
}

// CreateChatInstance creates a new chat instance for a profile+team combination.
func (s *Store) CreateChatInstance(profile, team string, skills []string) (*StandaloneChatInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := hexid.New()
	dir := s.instanceDir(id)
	if err := os.MkdirAll(filepath.Join(dir, "messages"), 0755); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	inst := &StandaloneChatInstance{
		ID:        id,
		Profile:   profile,
		Team:      team,
		Skills:    skills,
		Title:     "New Chat",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.writeJSON(filepath.Join(dir, "meta.json"), inst); err != nil {
		return nil, err
	}
	return inst, nil
}

// DeleteChatInstance removes a chat instance and all its messages.
func (s *Store) DeleteChatInstance(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.instanceDir(id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dir)
}

// UpdateChatInstanceTitle sets the title on a chat instance.
func (s *Store) UpdateChatInstanceTitle(id, title string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metaPath := filepath.Join(s.instanceDir(id), "meta.json")
	var inst StandaloneChatInstance
	if err := s.readJSON(metaPath, &inst); err != nil {
		return err
	}
	inst.Title = title
	inst.UpdatedAt = time.Now().UTC()
	return s.writeJSON(metaPath, &inst)
}

// UpdateChatInstanceLastSession stores the daemon session ID on a chat instance
// so that follow-up messages can resume the agent session.
func (s *Store) UpdateChatInstanceLastSession(id string, sessionID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metaPath := filepath.Join(s.instanceDir(id), "meta.json")
	var inst StandaloneChatInstance
	if err := s.readJSON(metaPath, &inst); err != nil {
		return err
	}
	inst.LastSessionID = sessionID
	inst.UpdatedAt = time.Now().UTC()
	return s.writeJSON(metaPath, &inst)
}

// ListChatInstanceMessages returns all messages for a chat instance.
func (s *Store) ListChatInstanceMessages(instanceID string) ([]StandaloneChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.instanceMessagesDir(instanceID)
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

// CreateChatInstanceMessage adds a message to a chat instance.
// It also auto-sets the instance title from the first user message and
// updates the instance's UpdatedAt timestamp.
func (s *Store) CreateChatInstanceMessage(instanceID string, msg *StandaloneChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgDir := s.instanceMessagesDir(instanceID)
	if err := os.MkdirAll(msgDir, 0755); err != nil {
		return err
	}

	msg.ID = s.nextID(msgDir)
	msg.CreatedAt = time.Now().UTC()
	filename := fmt.Sprintf("%d.json", msg.ID)
	if err := s.writeJSON(filepath.Join(msgDir, filename), msg); err != nil {
		return err
	}

	// Update instance meta (title from first user message + updatedAt)
	metaPath := filepath.Join(s.instanceDir(instanceID), "meta.json")
	var inst StandaloneChatInstance
	if err := s.readJSON(metaPath, &inst); err == nil {
		inst.UpdatedAt = time.Now().UTC()
		if inst.Title == "New Chat" && msg.Role == "user" && msg.Content != "" {
			title := msg.Content
			if len(title) > 80 {
				title = title[:80] + "\u2026"
			}
			inst.Title = title
		}
		_ = s.writeJSON(metaPath, &inst)
	}

	return nil
}
