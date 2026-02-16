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
	opencodeFixtureDir             = "testdata/opencode"
	opencodeFixtureRecordEnv       = "ADAF_RECORD_OPENCODE_FIXTURE"
	opencodeFixtureDemoRepoRoot    = "/tmp/adaf-opencode-demo-repo"
	opencodeFixtureSchemaVersionV1 = 1
)

var opencodeSemverRe = regexp.MustCompile(`\d+\.\d+\.\d+([A-Za-z0-9.+-]*)`)

type opencodeFixture struct {
	SchemaVersion int                          `json:"schema_version"`
	Provider      string                       `json:"provider"`
	Version       string                       `json:"version"`
	VersionRaw    string                       `json:"version_raw"`
	Binary        string                       `json:"binary"`
	CapturedAt    string                       `json:"captured_at"`
	Prompt        string                       `json:"prompt"`
	Args          []string                     `json:"args"`
	Result        opencodeFixtureResult        `json:"result"`
	Summary       opencodeFixtureReplaySummary `json:"summary"`
	Stream        []string                     `json:"stream"`
	Events        []opencodeFixtureEvent       `json:"events"`
	Workspace     map[string]string            `json:"workspace,omitempty"`
	Metadata      map[string]string            `json:"metadata,omitempty"`
}

type opencodeFixtureResult struct {
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	Error          string `json:"error"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
}

type opencodeFixtureEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type opencodeFixtureReplaySummary struct {
	RawEvents        int `json:"raw_events"`
	ParsedEvents     int `json:"parsed_events"`
	AssistantEvents  int `json:"assistant_events"`
	ThinkingBlocks   int `json:"thinking_blocks"`
	TextBlocks       int `json:"text_blocks"`
	ToolUseBlocks    int `json:"tool_use_blocks"`
	ToolResultBlocks int `json:"tool_result_blocks"`
	ResultEvents     int `json:"result_events"`
	ErrorEvents      int `json:"error_events"`
}

func mustReadOpencodeFixtures(t *testing.T) []opencodeFixture {
	t.Helper()

	entries, err := os.ReadDir(opencodeFixtureDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", opencodeFixtureDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" || !strings.HasPrefix(name, "opencode-") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	fixtures := make([]opencodeFixture, 0, len(names))
	for _, name := range names {
		path := filepath.Join(opencodeFixtureDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		var fixture opencodeFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("Unmarshal(%s): %v", path, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func writeOpencodeFixture(t *testing.T, fixture opencodeFixture) string {
	t.Helper()

	if err := os.MkdirAll(opencodeFixtureDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", opencodeFixtureDir, err)
	}
	path := filepath.Join(opencodeFixtureDir, "opencode-"+sanitizeFixtureToken(fixture.Version)+".json")

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

func detectOpencodeVersion(t *testing.T, binary string) (raw string, normalized string) {
	t.Helper()

	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("detecting opencode version: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	raw = strings.TrimSpace(string(out))
	normalized = normalizeOpencodeVersion(raw)
	return raw, normalized
}

func normalizeOpencodeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	if semver := opencodeSemverRe.FindString(raw); semver != "" {
		return sanitizeFixtureToken(semver)
	}
	line := strings.TrimSpace(strings.Split(raw, "\n")[0])
	return sanitizeFixtureToken(line)
}

func captureOpencodeFixtureEvents(events []store.RecordingEvent, workspace string) []opencodeFixtureEvent {
	out := make([]opencodeFixtureEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == "claude_stream" {
			continue
		}
		data := ev.Data
		if workspace != "" && strings.HasPrefix(data, "workdir="+workspace) {
			data = "workdir=" + opencodeFixtureDemoRepoRoot
		}
		out = append(out, opencodeFixtureEvent{
			Type: ev.Type,
			Data: data,
		})
	}
	return out
}

func opencodeFixtureNDJSON(fixture opencodeFixture) string {
	lines := fixture.Stream
	if len(lines) == 0 {
		lines = opencodeStreamLinesFromEvents(fixture.Events)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func opencodeStreamLinesFromEvents(events []opencodeFixtureEvent) []string {
	var lines []string
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		lines = append(lines, ev.Data)
	}
	return lines
}

func opencodeStreamLinesFromRecording(events []store.RecordingEvent) []string {
	var lines []string
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		lines = append(lines, ev.Data)
	}
	return lines
}

func summarizeOpencodeFixtureStream(ndjson string) (opencodeFixtureReplaySummary, string, error) {
	var summary opencodeFixtureReplaySummary
	if strings.TrimSpace(ndjson) == "" {
		return summary, "", fmt.Errorf("empty NDJSON stream")
	}

	ch := stream.ParseOpencode(context.Background(), strings.NewReader(ndjson))

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
					switch block.Type {
					case "thinking":
						summary.ThinkingBlocks++
					case "text":
						summary.TextBlocks++
					case "tool_use":
						summary.ToolUseBlocks++
					case "tool_result":
						summary.ToolResultBlocks++
					}
				}
			}
		case "result":
			summary.ResultEvents++
		case "error":
			summary.ErrorEvents++
		}
		defaultAccumulateText(ev.Parsed, &accumulated)
	}

	if firstErr != nil {
		return summary, strings.TrimSpace(accumulated.String()), firstErr
	}
	return summary, strings.TrimSpace(accumulated.String()), nil
}

func prepareOpencodeDemoRepo(t *testing.T) string {
	t.Helper()

	root := opencodeFixtureDemoRepoRoot
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
