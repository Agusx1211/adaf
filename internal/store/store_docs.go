// store_docs.go contains document management methods.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
