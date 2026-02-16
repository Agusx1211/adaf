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

// TestRecordCodexFixture captures a real Codex run and saves it under
// testdata/codex/codex-<version>.json for replay tests.
//
// Run explicitly:
//
//	ADAF_RECORD_CODEX_FIXTURE=1 go test -tags=integration ./internal/agent -run TestRecordCodexFixture -v
func TestRecordCodexFixture(t *testing.T) {
	if os.Getenv(codexFixtureRecordEnv) != "1" {
		t.Skipf("set %s=1 to record a Codex fixture", codexFixtureRecordEnv)
	}

	binary := findBinary("codex")
	if binary == "" {
		t.Skip("codex binary not found")
	}

	workspace := prepareCodexDemoRepo(t)

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: "codex-fixture-record"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	prompt := strings.Join([]string{
		"In the current directory, create a file named fixture_note.txt with exactly this content: CODEX_FIXTURE_FILE_CONTENT.",
		"Then answer with exactly: CODEX_FIXTURE_OK.",
	}, " ")
	args := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	result, err := NewCodexAgent().Run(ctx, Config{
		Command: binary,
		WorkDir: workspace,
		Args:    args,
		Prompt:  prompt,
	}, rec)
	if err != nil {
		t.Fatalf("NewCodexAgent().Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("NewCodexAgent().Run() returned nil result")
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d; stderr:\n%s\noutput:\n%s", result.ExitCode, result.Error, result.Output)
	}

	versionRaw, version := detectCodexVersion(t, binary)
	fixture := codexFixture{
		SchemaVersion: codexFixtureSchemaVersionV1,
		Provider:      "codex",
		Version:       version,
		VersionRaw:    versionRaw,
		Binary:        filepath.Base(binary),
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Prompt:        prompt,
		Args:          args,
		Result: codexFixtureResult{
			ExitCode:       result.ExitCode,
			Output:         strings.TrimSpace(result.Output),
			Error:          result.Error,
			AgentSessionID: result.AgentSessionID,
		},
		Stream:    codexStreamLinesFromRecording(rec.Events()),
		Events:    captureCodexFixtureEvents(rec.Events(), workspace),
		Workspace: collectWorkspaceSnapshot(t, workspace),
		Metadata: map[string]string{
			"demo_repo":  codexFixtureDemoRepoRoot,
			"record_env": codexFixtureRecordEnv,
		},
	}

	ndjson := codexFixtureNDJSON(fixture)
	summary, replayOutput, err := summarizeCodexFixtureStream(ndjson)
	if err != nil {
		t.Fatalf("summarizeCodexFixtureStream failed: %v", err)
	}
	fixture.Summary = summary

	if replayOutput != fixture.Result.Output {
		t.Fatalf("fixture replay output mismatch: replay=%q result=%q", replayOutput, fixture.Result.Output)
	}

	path := writeCodexFixture(t, fixture)
	t.Logf("codex fixture recorded: %s", path)
}
