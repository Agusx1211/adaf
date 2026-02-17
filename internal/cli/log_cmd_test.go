package cli

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestTurnCommand_RegistersFinishSubcommand(t *testing.T) {
	found, _, err := turnCmd.Find([]string{"finish"})
	if err != nil {
		t.Fatalf("turnCmd.Find(finish) error = %v", err)
	}
	if found == nil || found.Name() != "finish" {
		t.Fatalf("turn finish subcommand not registered")
	}
}

func TestTurnCommand_DoesNotRegisterUpdateSubcommand(t *testing.T) {
	for _, sub := range turnCmd.Commands() {
		if sub.Name() == "update" {
			t.Fatalf("turn update subcommand should not exist")
		}
	}
}

func TestRunTurnFinish_UsesADAFTurnID(t *testing.T) {
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

	cmd := newTurnFinishTestCommand(t)
	if err := cmd.Flags().Set("built", "added auth module"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := cmd.Flags().Set("decisions", "kept single-file approach"); err != nil {
		t.Fatalf("setting decisions flag: %v", err)
	}
	if err := cmd.Flags().Set("challenges", "fixed edge-case toggle behavior"); err != nil {
		t.Fatalf("setting challenges flag: %v", err)
	}
	if err := cmd.Flags().Set("state", "demo and controls now match spec"); err != nil {
		t.Fatalf("setting state flag: %v", err)
	}
	if err := cmd.Flags().Set("issues", "none"); err != nil {
		t.Fatalf("setting issues flag: %v", err)
	}
	if err := cmd.Flags().Set("next", "add refresh-token flow"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}

	if err := runTurnFinish(cmd, nil); err != nil {
		t.Fatalf("runTurnFinish() error = %v", err)
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
	if updated.KeyDecisions != "kept single-file approach" {
		t.Fatalf("KeyDecisions = %q, want %q", updated.KeyDecisions, "kept single-file approach")
	}
}

func TestRunTurnFinish_RequiresAllSections(t *testing.T) {
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

	cmd := newTurnFinishTestCommand(t)
	if err := cmd.Flags().Set("built", "added auth module"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := cmd.Flags().Set("decisions", "kept single-file approach"); err != nil {
		t.Fatalf("setting decisions flag: %v", err)
	}
	if err := cmd.Flags().Set("state", "demo and controls now match spec"); err != nil {
		t.Fatalf("setting state flag: %v", err)
	}
	if err := cmd.Flags().Set("issues", "none"); err != nil {
		t.Fatalf("setting issues flag: %v", err)
	}
	if err := cmd.Flags().Set("next", "prepare refactor follow-up"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}

	err = runTurnFinish(cmd, nil)
	if err == nil {
		t.Fatal("runTurnFinish() error = nil, want missing sections error")
	}
	if !strings.Contains(err.Error(), "--challenges") {
		t.Fatalf("runTurnFinish() error = %v, want missing --challenges flag in message", err)
	}
}

func TestRunTurnFinish_SecondCallOverridesPreviousFinish(t *testing.T) {
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

	first := newTurnFinishTestCommand(t)
	if err := first.Flags().Set("built", "v1 built"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := first.Flags().Set("decisions", "v1 decisions"); err != nil {
		t.Fatalf("setting decisions flag: %v", err)
	}
	if err := first.Flags().Set("challenges", "v1 challenges"); err != nil {
		t.Fatalf("setting challenges flag: %v", err)
	}
	if err := first.Flags().Set("state", "v1 state"); err != nil {
		t.Fatalf("setting state flag: %v", err)
	}
	if err := first.Flags().Set("issues", "v1 issues"); err != nil {
		t.Fatalf("setting issues flag: %v", err)
	}
	if err := first.Flags().Set("next", "v1 next"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}
	if err := runTurnFinish(first, nil); err != nil {
		t.Fatalf("first runTurnFinish() error = %v", err)
	}

	second := newTurnFinishTestCommand(t)
	if err := second.Flags().Set("built", "v2 built"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := second.Flags().Set("decisions", "v2 decisions"); err != nil {
		t.Fatalf("setting decisions flag: %v", err)
	}
	if err := second.Flags().Set("challenges", "v2 challenges"); err != nil {
		t.Fatalf("setting challenges flag: %v", err)
	}
	if err := second.Flags().Set("state", "v2 state"); err != nil {
		t.Fatalf("setting state flag: %v", err)
	}
	if err := second.Flags().Set("issues", "v2 issues"); err != nil {
		t.Fatalf("setting issues flag: %v", err)
	}
	if err := second.Flags().Set("next", "v2 next"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}

	out := captureStdout(t, func() {
		if err := runTurnFinish(second, nil); err != nil {
			t.Fatalf("second runTurnFinish() error = %v", err)
		}
	})
	if !strings.Contains(out, "overwrote the previous finish report") {
		t.Fatalf("second finish call should report override\noutput:\n%s", out)
	}

	updated, err := s.GetTurn(turn.ID)
	if err != nil {
		t.Fatalf("GetTurn(%d) error = %v", turn.ID, err)
	}
	if updated.WhatWasBuilt != "v2 built" {
		t.Fatalf("WhatWasBuilt = %q, want %q", updated.WhatWasBuilt, "v2 built")
	}
	if updated.NextSteps != "v2 next" {
		t.Fatalf("NextSteps = %q, want %q", updated.NextSteps, "v2 next")
	}
}

func TestRunTurnFinish_RejectsFrozenTurn(t *testing.T) {
	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "demo", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	turn := &store.Turn{
		Agent:       "codex",
		Objective:   "completed",
		FinalizedAt: time.Now().UTC(),
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

	cmd := newTurnFinishTestCommand(t)
	if err := cmd.Flags().Set("built", "should-fail"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := cmd.Flags().Set("decisions", "d"); err != nil {
		t.Fatalf("setting decisions flag: %v", err)
	}
	if err := cmd.Flags().Set("challenges", "c"); err != nil {
		t.Fatalf("setting challenges flag: %v", err)
	}
	if err := cmd.Flags().Set("state", "s"); err != nil {
		t.Fatalf("setting state flag: %v", err)
	}
	if err := cmd.Flags().Set("issues", "i"); err != nil {
		t.Fatalf("setting issues flag: %v", err)
	}
	if err := cmd.Flags().Set("next", "n"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}

	err = runTurnFinish(cmd, nil)
	if err == nil {
		t.Fatal("runTurnFinish() error = nil, want frozen turn error")
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("runTurnFinish() error = %v, want frozen message", err)
	}
}

func TestRunTurnFinish_RejectsTerminalBuildStateTurn(t *testing.T) {
	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "demo", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	turn := &store.Turn{
		Agent:      "codex",
		Objective:  "completed",
		BuildState: "success",
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

	cmd := newTurnFinishTestCommand(t)
	if err := cmd.Flags().Set("built", "should-fail"); err != nil {
		t.Fatalf("setting built flag: %v", err)
	}
	if err := cmd.Flags().Set("decisions", "d"); err != nil {
		t.Fatalf("setting decisions flag: %v", err)
	}
	if err := cmd.Flags().Set("challenges", "c"); err != nil {
		t.Fatalf("setting challenges flag: %v", err)
	}
	if err := cmd.Flags().Set("state", "s"); err != nil {
		t.Fatalf("setting state flag: %v", err)
	}
	if err := cmd.Flags().Set("issues", "i"); err != nil {
		t.Fatalf("setting issues flag: %v", err)
	}
	if err := cmd.Flags().Set("next", "n"); err != nil {
		t.Fatalf("setting next flag: %v", err)
	}

	err = runTurnFinish(cmd, nil)
	if err == nil {
		t.Fatal("runTurnFinish() error = nil, want frozen turn error")
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("runTurnFinish() error = %v, want frozen message", err)
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

func newTurnFinishTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("objective", "", "")
	cmd.Flags().String("built", "", "")
	cmd.Flags().String("decisions", "", "")
	cmd.Flags().String("challenges", "", "")
	cmd.Flags().String("state", "", "")
	cmd.Flags().String("issues", "", "")
	cmd.Flags().String("next", "", "")
	return cmd
}
