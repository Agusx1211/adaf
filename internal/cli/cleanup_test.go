package cli

import (
	"testing"

	"github.com/agusx1211/adaf/internal/store"
)

func TestDeadWorktreePathsIncludesCanceled(t *testing.T) {
	dir := t.TempDir()
	s, err := store.New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	createSpawn := func(status, path string) {
		t.Helper()
		rec := &store.SpawnRecord{
			ParentTurnID:  1,
			ParentProfile: "parent",
			ChildProfile:  "child",
			Task:          "task",
			Status:        status,
			WorktreePath:  path,
		}
		if err := s.CreateSpawn(rec); err != nil {
			t.Fatalf("CreateSpawn(%q) error = %v", status, err)
		}
	}

	createSpawn("completed", "/tmp/wt-completed")
	createSpawn("canceled", "/tmp/wt-canceled")
	createSpawn("cancelled", "/tmp/wt-cancelled")
	createSpawn("running", "/tmp/wt-running")
	got := deadWorktreePaths(s)

	for _, want := range []string{"/tmp/wt-completed", "/tmp/wt-canceled", "/tmp/wt-cancelled"} {
		if !got[want] {
			t.Fatalf("deadWorktreePaths missing %q", want)
		}
	}
	if got["/tmp/wt-running"] {
		t.Fatalf("deadWorktreePaths included non-terminal path %q", "/tmp/wt-running")
	}
}
