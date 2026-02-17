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
	claudeFixtureDir             = "testdata/claude"
	claudeFixtureRecordEnv       = "ADAF_RECORD_CLAUDE_FIXTURE"
	claudeFixtureDemoRepoRoot    = "/tmp/adaf-claude-demo-repo"
	claudeFixtureSchemaVersionV1 = 1
)

var claudeSemverRe = regexp.MustCompile(`\d+\.\d+\.\d+([A-Za-z0-9.+-]*)`)

type claudeFixture struct {
	SchemaVersion int                        `json:"schema_version"`
	Provider      string                     `json:"provider"`
	Version       string                     `json:"version"`
	VersionRaw    string                     `json:"version_raw"`
	Binary        string                     `json:"binary"`
	CapturedAt    string                     `json:"captured_at"`
	Prompt        string                     `json:"prompt"`
	Args          []string                   `json:"args"`
	Result        claudeFixtureResult        `json:"result"`
	Summary       claudeFixtureReplaySummary `json:"summary"`
	Stream        []string                   `json:"stream"`
	Events        []claudeFixtureEvent       `json:"events"`
	Workspace     map[string]string          `json:"workspace,omitempty"`
	Metadata      map[string]string          `json:"metadata,omitempty"`
}

type claudeFixtureResult struct {
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	Error          string `json:"error"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
}

type claudeFixtureEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type claudeFixtureReplaySummary struct {
	RawEvents        int `json:"raw_events"`
	ParsedEvents     int `json:"parsed_events"`
	SystemEvents     int `json:"system_events"`
	AssistantEvents  int `json:"assistant_events"`
	UserEvents       int `json:"user_events"`
	ResultEvents     int `json:"result_events"`
	ErrorEvents      int `json:"error_events"`
	TextBlocks       int `json:"text_blocks"`
	ThinkingBlocks   int `json:"thinking_blocks"`
	ToolUseBlocks    int `json:"tool_use_blocks"`
	ToolResultBlocks int `json:"tool_result_blocks"`
	DeltaEvents      int `json:"delta_events"`
}

func mustReadClaudeFixtures(t *testing.T) []claudeFixture {
	t.Helper()

	entries, err := os.ReadDir(claudeFixtureDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", claudeFixtureDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" || !strings.HasPrefix(name, "claude-") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	fixtures := make([]claudeFixture, 0, len(names))
	for _, name := range names {
		path := filepath.Join(claudeFixtureDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		var fixture claudeFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("Unmarshal(%s): %v", path, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func writeClaudeFixture(t *testing.T, fixture claudeFixture) string {
	t.Helper()

	if err := os.MkdirAll(claudeFixtureDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", claudeFixtureDir, err)
	}
	path := filepath.Join(claudeFixtureDir, "claude-"+sanitizeFixtureToken(fixture.Version)+".json")

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

func detectClaudeVersion(t *testing.T, binary string) (raw string, normalized string) {
	t.Helper()

	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("detecting claude version: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	raw = strings.TrimSpace(string(out))
	normalized = normalizeClaudeVersion(raw)
	return raw, normalized
}

func normalizeClaudeVersion(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	if semver := claudeSemverRe.FindString(raw); semver != "" {
		return sanitizeFixtureToken(semver)
	}
	line := strings.TrimSpace(strings.Split(raw, "\n")[0])
	return sanitizeFixtureToken(line)
}

func captureClaudeFixtureEvents(events []store.RecordingEvent, workspace string) []claudeFixtureEvent {
	out := make([]claudeFixtureEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == "claude_stream" {
			continue
		}
		data := ev.Data
		if workspace != "" && strings.HasPrefix(data, "workdir="+workspace) {
			data = "workdir=" + claudeFixtureDemoRepoRoot
		}
		out = append(out, claudeFixtureEvent{Type: ev.Type, Data: data})
	}
	return out
}

func claudeFixtureNDJSON(fixture claudeFixture) string {
	lines := fixture.Stream
	if len(lines) == 0 {
		lines = claudeStreamLinesFromEvents(fixture.Events)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func claudeStreamLinesFromEvents(events []claudeFixtureEvent) []string {
	var lines []string
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		lines = append(lines, ev.Data)
	}
	return lines
}

func claudeStreamLinesFromRecording(events []store.RecordingEvent) []string {
	var lines []string
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		lines = append(lines, ev.Data)
	}
	return lines
}

func summarizeClaudeFixtureStream(ndjson string) (claudeFixtureReplaySummary, string, error) {
	var summary claudeFixtureReplaySummary
	if strings.TrimSpace(ndjson) == "" {
		return summary, "", fmt.Errorf("empty NDJSON stream")
	}

	ch := stream.Parse(context.Background(), strings.NewReader(ndjson))

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
		case "system":
			summary.SystemEvents++
		case "assistant":
			summary.AssistantEvents++
			if ev.Parsed.AssistantMessage != nil {
				for _, block := range ev.Parsed.AssistantMessage.Content {
					switch block.Type {
					case "text":
						summary.TextBlocks++
					case "thinking":
						summary.ThinkingBlocks++
					case "tool_use":
						summary.ToolUseBlocks++
					case "tool_result":
						summary.ToolResultBlocks++
					}
				}
			}
		case "user":
			summary.UserEvents++
			if ev.Parsed.AssistantMessage != nil {
				for _, block := range ev.Parsed.AssistantMessage.Content {
					if block.Type == "tool_result" {
						summary.ToolResultBlocks++
					}
				}
			}
		case "result":
			summary.ResultEvents++
		case "error":
			summary.ErrorEvents++
		case "content_block_delta":
			summary.DeltaEvents++
		}

		defaultAccumulateText(ev.Parsed, &accumulated)
	}

	if firstErr != nil {
		return summary, strings.TrimSpace(accumulated.String()), firstErr
	}
	return summary, strings.TrimSpace(accumulated.String()), nil
}

func prepareClaudeDemoRepo(t *testing.T) string {
	t.Helper()

	root := claudeFixtureDemoRepoRoot
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
