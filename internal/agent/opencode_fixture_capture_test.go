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

// TestRecordOpencodeFixture captures a real OpenCode run and saves it under
// testdata/opencode/opencode-<version>.json for replay tests.
//
// Run explicitly:
//
//	ADAF_RECORD_OPENCODE_FIXTURE=1 go test -tags=integration ./internal/agent -run TestRecordOpencodeFixture -v
func TestRecordOpencodeFixture(t *testing.T) {
	if os.Getenv(opencodeFixtureRecordEnv) != "1" {
		t.Skipf("set %s=1 to record an OpenCode fixture", opencodeFixtureRecordEnv)
	}

	binary := findBinary("opencode")
	if binary == "" {
		t.Skip("opencode binary not found")
	}

	workspace := prepareOpencodeDemoRepo(t)

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: "opencode-fixture-record"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	prompt := strings.Join([]string{
		"In the current directory, create a file named fixture_note.txt with exactly this content: OPENCODE_FIXTURE_FILE_CONTENT.",
		"Then answer with exactly: OPENCODE_FIXTURE_OK.",
	}, " ")
	args := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	result, err := NewOpencodeAgent().Run(ctx, Config{
		Command: binary,
		WorkDir: workspace,
		Args:    args,
		Prompt:  prompt,
	}, rec)
	if err != nil {
		t.Fatalf("NewOpencodeAgent().Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("NewOpencodeAgent().Run() returned nil result")
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d; stderr:\n%s\noutput:\n%s", result.ExitCode, result.Error, result.Output)
	}

	versionRaw, version := detectOpencodeVersion(t, binary)
	fixture := opencodeFixture{
		SchemaVersion: opencodeFixtureSchemaVersionV1,
		Provider:      "opencode",
		Version:       version,
		VersionRaw:    versionRaw,
		Binary:        filepath.Base(binary),
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Prompt:        prompt,
		Args:          args,
		Result: opencodeFixtureResult{
			ExitCode:       result.ExitCode,
			Output:         strings.TrimSpace(result.Output),
			Error:          result.Error,
			AgentSessionID: result.AgentSessionID,
		},
		Stream:    opencodeStreamLinesFromRecording(rec.Events()),
		Events:    captureOpencodeFixtureEvents(rec.Events(), workspace),
		Workspace: collectWorkspaceSnapshot(t, workspace),
		Metadata: map[string]string{
			"demo_repo":  opencodeFixtureDemoRepoRoot,
			"record_env": opencodeFixtureRecordEnv,
		},
	}

	ndjson := opencodeFixtureNDJSON(fixture)
	summary, replayOutput, err := summarizeOpencodeFixtureStream(ndjson)
	if err != nil {
		t.Fatalf("summarizeOpencodeFixtureStream failed: %v", err)
	}
	fixture.Summary = summary

	if replayOutput != fixture.Result.Output {
		t.Fatalf("fixture replay output mismatch: replay=%q result=%q", replayOutput, fixture.Result.Output)
	}

	path := writeOpencodeFixture(t, fixture)
	t.Logf("opencode fixture recorded: %s", path)
}
