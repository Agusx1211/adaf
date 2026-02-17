package cli

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/profilescore"
	"github.com/agusx1211/adaf/internal/store"
)

func TestRunSpawnFeedbackRecordsFeedback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	t.Setenv("ADAF_PROJECT_DIR", projectDir)
	t.Setenv("ADAF_ROLE", "lead")
	t.Setenv("ADAF_POSITION", "manager")

	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test-project", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	spawn := &store.SpawnRecord{
		ParentTurnID:  99,
		ParentProfile: "manager-a",
		ChildProfile:  "spark",
		ChildRole:     "scout",
		ChildPosition: "worker",
		Task:          "Inspect flaky tests",
		Status:        store.SpawnStatusRunning,
	}
	if err := s.CreateSpawn(spawn); err != nil {
		t.Fatalf("CreateSpawn() error = %v", err)
	}
	updated, err := s.GetSpawn(spawn.ID)
	if err != nil {
		t.Fatalf("GetSpawn() error = %v", err)
	}
	updated.Status = store.SpawnStatusCompleted
	updated.StartedAt = time.Now().Add(-2 * time.Minute).UTC()
	updated.CompletedAt = time.Now().UTC()
	if err := s.UpdateSpawn(updated); err != nil {
		t.Fatalf("UpdateSpawn() error = %v", err)
	}

	cmd := newSpawnFeedbackTestCommand()
	_ = cmd.Flags().Set("spawn-id", "1")
	_ = cmd.Flags().Set("difficulty", "6")
	_ = cmd.Flags().Set("quality", "8")
	_ = cmd.Flags().Set("notes", "good result")

	if err := runSpawnFeedback(cmd, nil); err != nil {
		t.Fatalf("runSpawnFeedback() error = %v", err)
	}

	records, err := profilescore.Default().ListFeedback()
	if err != nil {
		t.Fatalf("ListFeedback() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("feedback records len = %d, want 1", len(records))
	}
	if records[0].ChildProfile != "spark" {
		t.Fatalf("child_profile = %q, want %q", records[0].ChildProfile, "spark")
	}
	if records[0].ParentRole != "lead" {
		t.Fatalf("parent_role = %q, want %q", records[0].ParentRole, "lead")
	}
}

func TestRunSpawnFeedbackRejectsNonTerminalSpawn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectDir := t.TempDir()
	t.Setenv("ADAF_PROJECT_DIR", projectDir)

	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "test-project", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	if err := s.CreateSpawn(&store.SpawnRecord{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "spark",
		Status:        store.SpawnStatusRunning,
		Task:          "do work",
	}); err != nil {
		t.Fatalf("CreateSpawn() error = %v", err)
	}

	cmd := newSpawnFeedbackTestCommand()
	_ = cmd.Flags().Set("spawn-id", "1")
	_ = cmd.Flags().Set("difficulty", "5")
	_ = cmd.Flags().Set("quality", "7")
	err = runSpawnFeedback(cmd, nil)
	if err == nil {
		t.Fatal("runSpawnFeedback() error = nil, want non-terminal spawn error")
	}
}

func TestRunSpawnFeedbackValidatesScoreRange(t *testing.T) {
	cmd := newSpawnFeedbackTestCommand()
	_ = cmd.Flags().Set("spawn-id", "1")
	_ = cmd.Flags().Set("difficulty", "15")
	_ = cmd.Flags().Set("quality", "7")
	err := runSpawnFeedback(cmd, nil)
	if err == nil {
		t.Fatal("runSpawnFeedback() error = nil, want validation error")
	}
}

func newSpawnFeedbackTestCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.Flags().Int("spawn-id", 0, "")
	cmd.Flags().Float64("difficulty", -1, "")
	cmd.Flags().Float64("quality", -1, "")
	cmd.Flags().String("notes", "", "")
	cmd.Flags().String("parent-role", "", "")
	cmd.Flags().String("parent-position", "", "")
	return cmd
}
