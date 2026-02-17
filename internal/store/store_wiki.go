// store_wiki.go contains wiki management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultWikiSearchLimit = 25

func (s *Store) ListWiki() ([]WikiEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.root, "wiki")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var wiki []WikiEntry
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var entry WikiEntry
		if err := s.readJSON(filepath.Join(dir, e.Name()), &entry); err != nil {
			continue
		}
		wiki = append(wiki, entry)
	}
	sort.Slice(wiki, func(i, j int) bool { return wiki[i].ID < wiki[j].ID })
	return wiki, nil
}

func (s *Store) ListWikiForPlan(planID string) ([]WikiEntry, error) {
	if planID == "" {
		return s.ListSharedWiki()
	}
	wiki, err := s.ListWiki()
	if err != nil {
		return nil, err
	}
	var filtered []WikiEntry
	for _, entry := range wiki {
		if entry.PlanID == "" || entry.PlanID == planID {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

func (s *Store) ListSharedWiki() ([]WikiEntry, error) {
	wiki, err := s.ListWiki()
	if err != nil {
		return nil, err
	}
	var filtered []WikiEntry
	for _, entry := range wiki {
		if entry.PlanID == "" {
			filtered = append(filtered, entry)
		}
	}
	return filtered, nil
}

func (s *Store) SearchWiki(query string, limit int) ([]WikiEntry, error) {
	wiki, err := s.ListWiki()
	if err != nil {
		return nil, err
	}
	if len(wiki) == 0 {
		return []WikiEntry{}, nil
	}

	if limit <= 0 {
		limit = defaultWikiSearchLimit
	}

	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	if normalizedQuery == "" {
		sort.Slice(wiki, func(i, j int) bool {
			if wiki[i].Updated.Equal(wiki[j].Updated) {
				return wiki[i].ID < wiki[j].ID
			}
			return wiki[i].Updated.After(wiki[j].Updated)
		})
		if len(wiki) > limit {
			wiki = wiki[:limit]
		}
		return wiki, nil
	}

	tokens := strings.Fields(normalizedQuery)
	type scoredEntry struct {
		entry WikiEntry
		score int
	}
	scored := make([]scoredEntry, 0, len(wiki))
	for _, entry := range wiki {
		score := wikiSearchScore(entry, normalizedQuery, tokens)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredEntry{entry: entry, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].entry.Updated.Equal(scored[j].entry.Updated) {
			return scored[i].entry.ID < scored[j].entry.ID
		}
		return scored[i].entry.Updated.After(scored[j].entry.Updated)
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}

	results := make([]WikiEntry, 0, len(scored))
	for _, match := range scored {
		results = append(results, match.entry)
	}
	return results, nil
}

func (s *Store) CreateWikiEntry(entry *WikiEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%d", s.nextID(filepath.Join(s.root, "wiki")))
	}

	now := time.Now().UTC()
	if entry.Created.IsZero() {
		entry.Created = now
	}
	entry.Created = entry.Created.UTC()
	entry.Updated = entry.Created

	actor := strings.TrimSpace(entry.UpdatedBy)
	if actor == "" {
		actor = strings.TrimSpace(entry.CreatedBy)
	}
	if actor != "" {
		entry.CreatedBy = actor
		entry.UpdatedBy = actor
	}

	if entry.Version <= 0 {
		entry.Version = 1
	}
	if len(entry.History) == 0 {
		entry.History = []WikiChange{{
			Version: entry.Version,
			Action:  "create",
			By:      actor,
			At:      entry.Created,
		}}
	}

	filename := entry.ID + ".json"
	if err := s.writeJSON(filepath.Join(s.root, "wiki", filename), entry); err != nil {
		return err
	}

	s.AutoCommit([]string{"wiki/" + filename}, fmt.Sprintf("adaf: create wiki %s", entry.ID))
	return nil
}

func (s *Store) GetWikiEntry(id string) (*WikiEntry, error) {
	var entry WikiEntry
	if err := s.readJSON(filepath.Join(s.root, "wiki", id+".json"), &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *Store) UpdateWikiEntry(entry *WikiEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	normalizeWikiEntry(entry, now)

	actor := strings.TrimSpace(entry.UpdatedBy)
	if actor == "" {
		actor = strings.TrimSpace(entry.CreatedBy)
	}

	entry.Updated = now
	if actor != "" {
		entry.UpdatedBy = actor
	}
	entry.Version++
	entry.History = append(entry.History, WikiChange{
		Version: entry.Version,
		Action:  "update",
		By:      actor,
		At:      now,
	})

	filename := entry.ID + ".json"
	if err := s.writeJSON(filepath.Join(s.root, "wiki", filename), entry); err != nil {
		return err
	}

	s.AutoCommit([]string{"wiki/" + filename}, fmt.Sprintf("adaf: update wiki %s", entry.ID))
	return nil
}

func (s *Store) DeleteWikiEntry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.root, "wiki", id+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("deleting wiki entry %q: %w", id, err)
	}

	s.AutoCommit([]string{"wiki/" + id + ".json"}, fmt.Sprintf("adaf: delete wiki %s", id))
	return nil
}

func normalizeWikiEntry(entry *WikiEntry, now time.Time) {
	if entry.Created.IsZero() {
		entry.Created = now
	}
	entry.Created = entry.Created.UTC()
	if entry.Updated.IsZero() {
		entry.Updated = entry.Created
	}
	entry.Updated = entry.Updated.UTC()

	if entry.Version < len(entry.History) {
		entry.Version = len(entry.History)
	}
	if entry.Version < 1 {
		entry.Version = 1
	}

	if entry.CreatedBy == "" {
		entry.CreatedBy = strings.TrimSpace(entry.UpdatedBy)
	}

	if len(entry.History) == 0 {
		entry.History = []WikiChange{{
			Version: 1,
			Action:  "create",
			By:      strings.TrimSpace(entry.CreatedBy),
			At:      entry.Created,
		}}
	}
}

func wikiSearchScore(entry WikiEntry, query string, tokens []string) int {
	if query == "" {
		return 0
	}

	id := strings.ToLower(entry.ID)
	title := strings.ToLower(entry.Title)
	content := strings.ToLower(entry.Content)
	if len(content) > 12000 {
		content = content[:12000]
	}

	score := 0
	switch {
	case id == query:
		score += 240
	case title == query:
		score += 220
	}
	if strings.Contains(id, query) {
		score += 140
	}
	if strings.Contains(title, query) {
		score += 130
	}
	if strings.Contains(content, query) {
		score += 48
	}

	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(id, token) {
			score += 40
		}
		if strings.Contains(title, token) {
			score += 30
		}
		if strings.Contains(content, token) {
			score += 10
		}
	}

	score += fuzzySubsequenceScore(query, id+" "+title)
	if score == 0 {
		score += fuzzySubsequenceScore(query, title+" "+content)
	}
	return score
}

func fuzzySubsequenceScore(query, text string) int {
	if query == "" || text == "" {
		return 0
	}
	q := []rune(query)
	t := []rune(text)
	qi := 0
	run := 0
	score := 0
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if q[qi] != t[ti] {
			run = 0
			continue
		}
		qi++
		run++
		score += 2 + run
	}
	if qi != len(q) {
		return 0
	}
	return score
}
