package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestRecordFailedAgentCommand_AgentContextRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ADAF_AGENT", "1")
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")

	recordFailedAgentCommand(errors.New(`unknown command "spwn" for "adaf"`), []string{"adaf", "spwn"})

	dir := filepath.Join(config.Dir(), "failed-commands")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	if len(entries) != 1 {
		t.Fatalf("failed command files = %d, want 1", len(entries))
	}
}

func TestRecordFailedAgentCommand_SkipsOutsideAgentContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ADAF_AGENT", "")
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")

	recordFailedAgentCommand(errors.New(`unknown command "spwn" for "adaf"`), []string{"adaf", "spwn"})

	dir := filepath.Join(config.Dir(), "failed-commands")
	_, err := os.Stat(dir)
	if !os.IsNotExist(err) {
		t.Fatalf("failed-commands dir should not exist, stat err = %v", err)
	}
}

func TestRecordFailedAgentCommand_SkipsRuntimeErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ADAF_AGENT", "1")
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")

	recordFailedAgentCommand(errors.New("spawn failed: daemon request failed"), []string{"adaf", "spawn"})

	dir := filepath.Join(config.Dir(), "failed-commands")
	_, err := os.Stat(dir)
	if !os.IsNotExist(err) {
		t.Fatalf("failed-commands dir should not exist for runtime errors, stat err = %v", err)
	}
}
