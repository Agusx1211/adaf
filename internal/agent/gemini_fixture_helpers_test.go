package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

const (
	geminiFixtureDir             = "testdata/gemini"
	geminiFixtureRecordEnv       = "ADAF_RECORD_GEMINI_FIXTURE"
	geminiFixtureDemoRepoRoot    = "/tmp/adaf-gemini-demo-repo"
	geminiFixtureSchemaVersionV1 = 1
)

var geminiSemverRe = regexp.MustCompile(`\d+\.\d+\.\d+([A-Za-z0-9.+-]*)`)

type geminiFixture struct {
	SchemaVersion int                        `json:"schema_version"`
	Provider      string                     `json:"provider"`
	Version       string                     `json:"version"`
	VersionRaw    string                     `json:"version_raw"`
	Binary        string                     `json:"binary"`
	CapturedAt    string                     `json:"captured_at"`
	Prompt        string                     `json:"prompt"`
	Args          []string                   `json:"args"`
	Result        geminiFixtureResult        `json:"result"`
	Summary       geminiFixtureReplaySummary `json:"summary"`
	Stream        []string                   `json:"stream"`
	Events        []geminiFixtureEvent       `json:"events"`
	Workspace     map[string]string          `json:"workspace,omitempty"`
	Metadata      map[string]string          `json:"metadata,omitempty"`
}

type geminiFixtureResult struct {
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	Error          string `json:"error"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
}

type geminiFixtureEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type geminiFixtureReplaySummary struct {
	RawEvents        int `json:"raw_events"`
	ParsedEvents     int `json:"parsed_events"`
	AssistantEvents  int `json:"assistant_events"`
	DeltaEvents      int `json:"delta_events"`
	ToolUseBlocks    int `json:"tool_use_blocks"`
	ToolResultBlocks int `json:"tool_result_blocks"`
	ResultEvents     int `json:"result_events"`
}

func mustReadGeminiFixtures(t *testing.T) []geminiFixture {
	t.Helper()

	entries, err := os.ReadDir(geminiFixtureDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", geminiFixtureDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" || !strings.HasPrefix(name, "gemini-") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	fixtures := make([]geminiFixture, 0, len(names))
	for _, name := range names {
		path := filepath.Join(geminiFixtureDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		var fixture geminiFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("Unmarshal(%s): %v", path, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func writeGeminiFixture(t *testing.T, fixture geminiFixture) string {
	t.Helper()

	if err := os.MkdirAll(geminiFixtureDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", geminiFixtureDir, err)
	}
	path := filepath.Join(geminiFixtureDir, "gemini-"+sanitizeFixtureToken(fixture.Version)+".json")

	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent(fixture): %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}

	return path
}

func detectGeminiVersion(t *testing.T, binary string) (raw string, normalized string) {
	t.Helper()

	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("detecting gemini version: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	raw = strings.TrimSpace(string(out))
	normalized = normalizeGeminiVersion(raw)
	return raw, normalized
}

func normalizeGeminiVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	if semver := geminiSemverRe.FindString(raw); semver != "" {
		return sanitizeFixtureToken(semver)
	}
	line := strings.TrimSpace(strings.Split(raw, "\n")[0])
	return sanitizeFixtureToken(line)
}

func captureGeminiFixtureEvents(events []store.RecordingEvent, workspace string) []geminiFixtureEvent {
	out := make([]geminiFixtureEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == "claude_stream" {
			continue
		}
		data := ev.Data
		if workspace != "" && strings.HasPrefix(data, "workdir="+workspace) {
			data = "workdir=" + geminiFixtureDemoRepoRoot
		}
		out = append(out, geminiFixtureEvent{
			Type: ev.Type,
			Data: data,
		})
	}
	return out
}

func geminiFixtureNDJSON(fixture geminiFixture) string {
	lines := fixture.Stream
	if len(lines) == 0 {
		lines = geminiStreamLinesFromEvents(fixture.Events)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func geminiStreamLinesFromEvents(events []geminiFixtureEvent) []string {
	var lines []string
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		lines = append(lines, ev.Data)
	}
	return lines
}

func geminiStreamLinesFromRecording(events []store.RecordingEvent) []string {
	var lines []string
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		lines = append(lines, ev.Data)
	}
	return lines
}

func summarizeGeminiFixtureStream(ndjson string) (geminiFixtureReplaySummary, string, error) {
	var summary geminiFixtureReplaySummary
	if strings.TrimSpace(ndjson) == "" {
		return summary, "", fmt.Errorf("empty NDJSON stream")
	}

	ch := stream.ParseGemini(context.Background(), strings.NewReader(ndjson))

	var (
		accumulated strings.Builder
		firstErr    error
	)
	for ev := range ch {
		summary.RawEvents++
		if ev.Err != nil && firstErr == nil {
			firstErr = ev.Err
		}
		if ev.Parsed.Type == "" {
			continue
		}
		summary.ParsedEvents++
		switch ev.Parsed.Type {
		case "assistant":
			if ev.Parsed.AssistantMessage != nil {
				summary.AssistantEvents++
				for _, block := range ev.Parsed.AssistantMessage.Content {
					if block.Type == "tool_use" {
						summary.ToolUseBlocks++
					}
				}
			}
		case "user":
			if ev.Parsed.AssistantMessage != nil {
				for _, block := range ev.Parsed.AssistantMessage.Content {
					if block.Type == "tool_result" {
						summary.ToolResultBlocks++
					}
				}
			}
		case "content_block_delta":
			summary.DeltaEvents++
		case "result":
			summary.ResultEvents++
		}
		defaultAccumulateText(ev.Parsed, &accumulated)
	}

	if firstErr != nil {
		return summary, strings.TrimSpace(accumulated.String()), firstErr
	}
	return summary, strings.TrimSpace(accumulated.String()), nil
}

func prepareGeminiDemoRepo(t *testing.T) string {
	t.Helper()

	root := geminiFixtureDemoRepoRoot
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll(%s): %v", root, err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", root, err)
	}

	runCmd := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	runCmd("git", "init")
	runCmd("git", "config", "user.email", "fixture@test.local")
	runCmd("git", "config", "user.name", "Fixture Generator")

	commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "init fixture repo")
	commitCmd.Dir = root
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
	)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit --allow-empty failed: %v\n%s", err, out)
	}

	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return root
}
