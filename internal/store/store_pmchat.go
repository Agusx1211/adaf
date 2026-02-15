package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s *Store) ListPMChatMessages() ([]PMChatMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.localDir("pmchat")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var messages []PMChatMessage
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var msg PMChatMessage
		if err := s.readJSON(filepath.Join(dir, e.Name()), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	sort.Slice(messages, func(i, j int) bool { return messages[i].ID < messages[j].ID })
	return messages, nil
}

func (s *Store) CreatePMChatMessage(msg *PMChatMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.localDir("pmchat")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	msg.ID = s.nextID(dir)
	msg.CreatedAt = time.Now().UTC()
	filename := fmt.Sprintf("%d.json", msg.ID)
	return s.writeJSON(filepath.Join(dir, filename), msg)
}

func (s *Store) ClearPMChatMessages() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.localDir("pmchat")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
