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
	vibeFixtureDir             = "testdata/vibe"
	vibeFixtureRecordEnv       = "ADAF_RECORD_VIBE_FIXTURE"
	vibeFixtureDemoRepoRoot    = "/tmp/adaf-vibe-demo-repo"
	vibeFixtureSchemaVersionV1 = 1
)

var (
	vibeSemverRe        = regexp.MustCompile(`\d+\.\d+\.\d+([A-Za-z0-9.+-]*)`)
	vibeSanitizeToken   = regexp.MustCompile(`[^a-z0-9._-]+`)
	vibeMultiUnderscore = regexp.MustCompile(`_+`)
)

type vibeFixture struct {
	SchemaVersion int                      `json:"schema_version"`
	Provider      string                   `json:"provider"`
	Version       string                   `json:"version"`
	VersionRaw    string                   `json:"version_raw"`
	Binary        string                   `json:"binary"`
	CapturedAt    string                   `json:"captured_at"`
	Prompt        string                   `json:"prompt"`
	Args          []string                 `json:"args"`
	Result        vibeFixtureResult        `json:"result"`
	Summary       vibeFixtureReplaySummary `json:"summary"`
	Stream        []string                 `json:"stream"`
	Events        []vibeFixtureEvent       `json:"events"`
	Workspace     map[string]string        `json:"workspace,omitempty"`
	Metadata      map[string]string        `json:"metadata,omitempty"`
}

type vibeFixtureResult struct {
	ExitCode       int    `json:"exit_code"`
	Output         string `json:"output"`
	Error          string `json:"error"`
	AgentSessionID string `json:"agent_session_id,omitempty"`
}

type vibeFixtureEvent struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type vibeFixtureReplaySummary struct {
	RawEvents        int `json:"raw_events"`
	ParsedEvents     int `json:"parsed_events"`
	AssistantEvents  int `json:"assistant_events"`
	ToolUseBlocks    int `json:"tool_use_blocks"`
	ToolResultBlocks int `json:"tool_result_blocks"`
}

func mustReadVibeFixtures(t *testing.T) []vibeFixture {
	t.Helper()

	entries, err := os.ReadDir(vibeFixtureDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", vibeFixtureDir, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".json" || !strings.HasPrefix(name, "vibe-") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	fixtures := make([]vibeFixture, 0, len(names))
	for _, name := range names {
		path := filepath.Join(vibeFixtureDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		var fixture vibeFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("Unmarshal(%s): %v", path, err)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func writeVibeFixture(t *testing.T, fixture vibeFixture) string {
	t.Helper()

	if err := os.MkdirAll(vibeFixtureDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", vibeFixtureDir, err)
	}
	path := filepath.Join(vibeFixtureDir, "vibe-"+sanitizeFixtureToken(fixture.Version)+".json")

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

func detectVibeVersion(t *testing.T, binary string) (raw string, normalized string) {
	t.Helper()

	out, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		altOut, altErr := exec.Command(binary, "version").CombinedOutput()
		if altErr == nil && len(strings.TrimSpace(string(altOut))) > 0 {
			out = altOut
			err = nil
		}
	}

	if err != nil {
		t.Fatalf("detecting vibe version: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	raw = strings.TrimSpace(string(out))
	normalized = normalizeVibeVersion(raw)
	return raw, normalized
}

func normalizeVibeVersion(raw string) string {
	line := strings.TrimSpace(strings.Split(raw, "\n")[0])
	if line == "" {
		return "unknown"
	}
	if semver := vibeSemverRe.FindString(line); semver != "" {
		return sanitizeFixtureToken(semver)
	}
	return sanitizeFixtureToken(line)
}

func sanitizeFixtureToken(value string) string {
	token := strings.ToLower(strings.TrimSpace(value))
	token = vibeSanitizeToken.ReplaceAllString(token, "_")
	token = vibeMultiUnderscore.ReplaceAllString(token, "_")
	token = strings.Trim(token, "._-")
	if token == "" {
		return "unknown"
	}
	return token
}

func captureFixtureEvents(events []store.RecordingEvent, workspace string) []vibeFixtureEvent {
	out := make([]vibeFixtureEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == "claude_stream" {
			continue
		}
		data := ev.Data
		if workspace != "" && strings.HasPrefix(data, "workdir="+workspace) {
			data = "workdir=" + vibeFixtureDemoRepoRoot
		}
		out = append(out, vibeFixtureEvent{
			Type: ev.Type,
			Data: data,
		})
	}
	return out
}

func fixtureNDJSON(fixture vibeFixture) string {
	lines := fixture.Stream
	if len(lines) == 0 {
		lines = dedupeStreamLinesFromEvents(fixture.Events)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func dedupeStreamLinesFromEvents(events []vibeFixtureEvent) []string {
	var lines []string
	last := ""
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		if len(lines) > 0 && ev.Data == last {
			continue
		}
		lines = append(lines, ev.Data)
		last = ev.Data
	}
	return lines
}

func dedupeStreamLinesFromRecording(events []store.RecordingEvent) []string {
	var lines []string
	last := ""
	for _, ev := range events {
		if ev.Type != "claude_stream" {
			continue
		}
		if len(lines) > 0 && ev.Data == last {
			continue
		}
		lines = append(lines, ev.Data)
		last = ev.Data
	}
	return lines
}

func summarizeVibeFixtureStream(ndjson string) (vibeFixtureReplaySummary, string, error) {
	var summary vibeFixtureReplaySummary
	if strings.TrimSpace(ndjson) == "" {
		return summary, "", fmt.Errorf("empty NDJSON stream")
	}

	ch := stream.ParseVibe(context.Background(), strings.NewReader(ndjson))

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
		if ev.Parsed.Type == "assistant" && ev.Parsed.AssistantMessage != nil {
			summary.AssistantEvents++
			for _, block := range ev.Parsed.AssistantMessage.Content {
				if block.Type == "tool_use" {
					summary.ToolUseBlocks++
				}
			}
		}
		if ev.Parsed.Type == "user" && ev.Parsed.AssistantMessage != nil {
			for _, block := range ev.Parsed.AssistantMessage.Content {
				if block.Type == "tool_result" {
					summary.ToolResultBlocks++
				}
			}
		}
		defaultAccumulateText(ev.Parsed, &accumulated)
	}

	if firstErr != nil {
		return summary, strings.TrimSpace(accumulated.String()), firstErr
	}
	return summary, strings.TrimSpace(accumulated.String()), nil
}

func collectWorkspaceSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()

	snapshot := make(map[string]string)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if rel == ".git" || rel == ".adaf" {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("collectWorkspaceSnapshot(%s): %v", root, err)
	}

	return snapshot
}

func prepareDemoRepo(t *testing.T) string {
	t.Helper()

	root := vibeFixtureDemoRepoRoot
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
