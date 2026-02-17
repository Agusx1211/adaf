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

// TestRecordVibeFixture captures a real Vibe run and saves it under
// testdata/vibe/vibe-<version>.json for replay tests.
//
// Run explicitly:
//
//	ADAF_RECORD_VIBE_FIXTURE=1 go test -tags=integration ./internal/agent -run TestRecordVibeFixture -v
func TestRecordVibeFixture(t *testing.T) {
	if os.Getenv(vibeFixtureRecordEnv) != "1" {
		t.Skipf("set %s=1 to record a Vibe fixture", vibeFixtureRecordEnv)
	}

	binary := findBinary("vibe")
	if binary == "" {
		t.Skip("vibe binary not found")
	}

	workspace := prepareDemoRepo(t)

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: "vibe-fixture-record"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	prompt := strings.Join([]string{
		"In the current directory, create a file named fixture_note.txt with exactly this content: VIBE_FIXTURE_FILE_CONTENT.",
		"Then answer with exactly: VIBE_FIXTURE_OK.",
	}, " ")
	args := []string{"--max-turns", "6"}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	result, err := NewVibeAgent().Run(ctx, Config{
		Command: binary,
		WorkDir: workspace,
		Args:    args,
		Prompt:  prompt,
	}, rec)
	if err != nil {
		t.Fatalf("NewVibeAgent().Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("NewVibeAgent().Run() returned nil result")
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d; stderr:\n%s\noutput:\n%s", result.ExitCode, result.Error, result.Output)
	}

	versionRaw, version := detectVibeVersion(t, binary)
	fixture := vibeFixture{
		SchemaVersion: vibeFixtureSchemaVersionV1,
		Provider:      "vibe",
		Version:       version,
		VersionRaw:    versionRaw,
		Binary:        filepath.Base(binary),
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Prompt:        prompt,
		Args:          args,
		Result: vibeFixtureResult{
			ExitCode:       result.ExitCode,
			Output:         strings.TrimSpace(result.Output),
			Error:          result.Error,
			AgentSessionID: result.AgentSessionID,
		},
		Stream:    streamLinesFromRecording(rec.Events()),
		Events:    captureFixtureEvents(rec.Events(), workspace),
		Workspace: collectWorkspaceSnapshot(t, workspace),
		Metadata: map[string]string{
			"demo_repo":  vibeFixtureDemoRepoRoot,
			"record_env": vibeFixtureRecordEnv,
		},
	}

	ndjson := fixtureNDJSON(fixture)
	summary, replayOutput, err := summarizeVibeFixtureStream(ndjson)
	if err != nil {
		t.Fatalf("summarizeVibeFixtureStream failed: %v", err)
	}
	fixture.Summary = summary

	if replayOutput != fixture.Result.Output {
		t.Fatalf("fixture replay output mismatch: replay=%q result=%q", replayOutput, fixture.Result.Output)
	}

	path := writeVibeFixture(t, fixture)
	t.Logf("vibe fixture recorded: %s", path)
}
