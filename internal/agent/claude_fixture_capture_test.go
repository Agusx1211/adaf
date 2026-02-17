//go:build integration

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

// TestRecordClaudeFixture captures a real Claude run and saves it under
// testdata/claude/claude-<version>.json for replay tests.
//
// Run explicitly:
//
//	ADAF_RECORD_CLAUDE_FIXTURE=1 go test -tags=integration ./internal/agent -run TestRecordClaudeFixture -v
func TestRecordClaudeFixture(t *testing.T) {
	if os.Getenv(claudeFixtureRecordEnv) != "1" {
		t.Skipf("set %s=1 to record a Claude fixture", claudeFixtureRecordEnv)
	}

	binary := findBinary("claude")
	if binary == "" {
		t.Skip("claude binary not found")
	}

	workspace := prepareClaudeDemoRepo(t)

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: "claude-fixture-record"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	prompt := strings.Join([]string{
		"In the current directory, create a file named fixture_note.txt with exactly this content: CLAUDE_FIXTURE_FILE_CONTENT.",
		"Then answer with exactly: CLAUDE_FIXTURE_OK.",
	}, " ")
	args := []string{"--dangerously-skip-permissions", "--max-turns", "5"}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	result, err := NewClaudeAgent().Run(ctx, Config{
		Command: binary,
		WorkDir: workspace,
		Args:    args,
		Prompt:  prompt,
	}, rec)
	if err != nil {
		t.Fatalf("NewClaudeAgent().Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("NewClaudeAgent().Run() returned nil result")
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d; stderr:\n%s\noutput:\n%s", result.ExitCode, result.Error, result.Output)
	}

	versionRaw, version := detectClaudeVersion(t, binary)
	fixture := claudeFixture{
		SchemaVersion: claudeFixtureSchemaVersionV1,
		Provider:      "claude",
		Version:       version,
		VersionRaw:    versionRaw,
		Binary:        filepath.Base(binary),
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Prompt:        prompt,
		Args:          args,
		Result: claudeFixtureResult{
			ExitCode:       result.ExitCode,
			Output:         strings.TrimSpace(result.Output),
			Error:          result.Error,
			AgentSessionID: result.AgentSessionID,
		},
		Stream:    claudeStreamLinesFromRecording(rec.Events()),
		Events:    captureClaudeFixtureEvents(rec.Events(), workspace),
		Workspace: collectWorkspaceSnapshot(t, workspace),
		Metadata: map[string]string{
			"demo_repo":  claudeFixtureDemoRepoRoot,
			"record_env": claudeFixtureRecordEnv,
		},
	}

	ndjson := claudeFixtureNDJSON(fixture)
	summary, replayOutput, err := summarizeClaudeFixtureStream(ndjson)
	if err != nil {
		t.Fatalf("summarizeClaudeFixtureStream failed: %v", err)
	}
	fixture.Summary = summary

	if replayOutput != fixture.Result.Output {
		t.Fatalf("fixture replay output mismatch: replay=%q result=%q", replayOutput, fixture.Result.Output)
	}

	path := writeClaudeFixture(t, fixture)
	t.Logf("claude fixture recorded: %s", path)
}
