package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/store"
)

func TestRunInitRepairsExistingProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()

	s, err := store.New(repo)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "demo", RepoPath: repo}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	missingDir := filepath.Join(s.Root(), "local", "stats", "loops")
	if err := os.RemoveAll(missingDir); err != nil {
		t.Fatalf("RemoveAll(%q) error = %v", missingDir, err)
	}
	if _, err := os.Stat(missingDir); !os.IsNotExist(err) {
		t.Fatalf("expected %q to be missing before repair", missingDir)
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("repo", ".", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("repo", repo); err != nil {
		t.Fatalf("setting repo flag: %v", err)
	}

	if err := runInit(cmd, nil); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	if _, err := os.Stat(missingDir); err != nil {
		t.Fatalf("expected %q to be recreated, stat err=%v", missingDir, err)
	}
}
