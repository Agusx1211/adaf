package cli

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

func TestTurnCommand_DefaultRunListsTurns(t *testing.T) {
	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "demo", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", projectDir, err)
	}

	out := captureStdout(t, func() {
		if err := turnCmd.RunE(turnCmd, nil); err != nil {
			t.Fatalf("turnCmd.RunE() error = %v", err)
		}
	})

	if !strings.Contains(out, "No turns found.") {
		t.Fatalf("default turn command should list turns, output:\n%s", out)
	}
	if strings.Contains(out, "Usage:") {
		t.Fatalf("default turn command should not print help, output:\n%s", out)
	}
}

func TestTurnCommand_RegistersUpdateSubcommand(t *testing.T) {
	found, _, err := turnCmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("turnCmd.Find(update) error = %v", err)
	}
	if found == nil || found.Name() != "update" {
		t.Fatalf("turn update subcommand not registered")
	}
}

func TestRunTurnUpdate_UsesADAFTurnID(t *testing.T) {
	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "demo", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	turn := &store.Turn{
		Agent:     "codex",
		Objective: "initial objective",
	}
	if err := s.CreateTurn(turn); err != nil {
		t.Fatalf("CreateTurn() error = %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("os.Chdir(%q) error = %v", projectDir, err)
	}
	t.Setenv("ADAF_TURN_ID", strconv.Itoa(turn.ID))

	cmd := newTurnUpdateTestCommand(t)
	if err := cmd.Flags().Set("built", "added auth module"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := cmd.Flags().Set("next", "add refresh-token flow"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}
	if err := cmd.Flags().Set("duration", "42"); err != nil {
		t.Fatalf("setting duration flag: %v", err)
	}

	if err := runTurnUpdate(cmd, nil); err != nil {
		t.Fatalf("runTurnUpdate() error = %v", err)
	}

	updated, err := s.GetTurn(turn.ID)
	if err != nil {
		t.Fatalf("GetTurn(%d) error = %v", turn.ID, err)
	}
	if updated.WhatWasBuilt != "added auth module" {
		t.Fatalf("WhatWasBuilt = %q, want %q", updated.WhatWasBuilt, "added auth module")
	}
	if updated.NextSteps != "add refresh-token flow" {
		t.Fatalf("NextSteps = %q, want %q", updated.NextSteps, "add refresh-token flow")
	}
	if updated.DurationSecs != 42 {
		t.Fatalf("DurationSecs = %d, want 42", updated.DurationSecs)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = w

	defer func() {
		_ = w.Close()
		os.Stdout = origStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing stdout writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading captured stdout: %v", err)
	}
	return string(data)
}

func newTurnUpdateTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("objective", "", "")
	cmd.Flags().String("built", "", "")
	cmd.Flags().String("decisions", "", "")
	cmd.Flags().String("challenges", "", "")
	cmd.Flags().String("state", "", "")
	cmd.Flags().String("issues", "", "")
	cmd.Flags().String("next", "", "")
	cmd.Flags().String("build-state", "", "")
	cmd.Flags().Int("duration", 0, "")
	return cmd
}
