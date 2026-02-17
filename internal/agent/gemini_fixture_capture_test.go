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

// TestRecordGeminiFixture captures a real Gemini run and saves it under
// testdata/gemini/gemini-<version>.json for replay tests.
//
// Run explicitly:
//
//	ADAF_RECORD_GEMINI_FIXTURE=1 go test -tags=integration ./internal/agent -run TestRecordGeminiFixture -v
func TestRecordGeminiFixture(t *testing.T) {
	if os.Getenv(geminiFixtureRecordEnv) != "1" {
		t.Skipf("set %s=1 to record a Gemini fixture", geminiFixtureRecordEnv)
	}

	binary := findBinary("gemini")
	if binary == "" {
		t.Skip("gemini binary not found")
	}

	workspace := prepareGeminiDemoRepo(t)

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: "gemini-fixture-record"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	prompt := strings.Join([]string{
		"In the current directory, create a file named fixture_note.txt with exactly this content: GEMINI_FIXTURE_FILE_CONTENT.",
		"Then answer with exactly: GEMINI_FIXTURE_OK.",
	}, " ")
	args := []string{"-y"}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	result, err := NewGeminiAgent().Run(ctx, Config{
		Command: binary,
		WorkDir: workspace,
		Args:    args,
		Prompt:  prompt,
	}, rec)
	if err != nil {
		t.Fatalf("NewGeminiAgent().Run() error: %v", err)
	}
	if result == nil {
		t.Fatal("NewGeminiAgent().Run() returned nil result")
	}
	if result.ExitCode != 0 {
		t.Fatalf("unexpected exit code %d; stderr:\n%s\noutput:\n%s", result.ExitCode, result.Error, result.Output)
	}

	versionRaw, version := detectGeminiVersion(t, binary)
	fixture := geminiFixture{
		SchemaVersion: geminiFixtureSchemaVersionV1,
		Provider:      "gemini",
		Version:       version,
		VersionRaw:    versionRaw,
		Binary:        filepath.Base(binary),
		CapturedAt:    time.Now().UTC().Format(time.RFC3339),
		Prompt:        prompt,
		Args:          args,
		Result: geminiFixtureResult{
			ExitCode:       result.ExitCode,
			Output:         strings.TrimSpace(result.Output),
			Error:          result.Error,
			AgentSessionID: result.AgentSessionID,
		},
		Stream:    geminiStreamLinesFromRecording(rec.Events()),
		Events:    captureGeminiFixtureEvents(rec.Events(), workspace),
		Workspace: collectWorkspaceSnapshot(t, workspace),
		Metadata: map[string]string{
			"demo_repo":  geminiFixtureDemoRepoRoot,
			"record_env": geminiFixtureRecordEnv,
		},
	}

	ndjson := geminiFixtureNDJSON(fixture)
	summary, replayOutput, err := summarizeGeminiFixtureStream(ndjson)
	if err != nil {
		t.Fatalf("summarizeGeminiFixtureStream failed: %v", err)
	}
	fixture.Summary = summary

	if replayOutput != fixture.Result.Output {
		t.Fatalf("fixture replay output mismatch: replay=%q result=%q", replayOutput, fixture.Result.Output)
	}

	path := writeGeminiFixture(t, fixture)
	t.Logf("gemini fixture recorded: %s", path)
}
