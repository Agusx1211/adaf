package agent

import (
	"strings"
	"testing"
)

func TestClaudeFixtureReplay(t *testing.T) {
	fixtures := mustReadClaudeFixtures(t)
	if len(fixtures) == 0 {
		t.Skipf("no Claude fixtures found in %s; run %s=1 go test -tags=integration ./internal/agent -run TestRecordClaudeFixture -v", claudeFixtureDir, claudeFixtureRecordEnv)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run("claude-"+fixture.Version, func(t *testing.T) {
			if fixture.SchemaVersion != claudeFixtureSchemaVersionV1 {
				t.Fatalf("unsupported schema_version=%d", fixture.SchemaVersion)
			}
			if fixture.Provider != "claude" {
				t.Fatalf("fixture provider=%q, want %q", fixture.Provider, "claude")
			}

			ndjson := claudeFixtureNDJSON(fixture)
			if strings.TrimSpace(ndjson) == "" {
				t.Fatal("fixture has no claude_stream events")
			}

			gotSummary, gotOutput, err := summarizeClaudeFixtureStream(ndjson)
			if err != nil {
				t.Fatalf("summarizeClaudeFixtureStream() error: %v", err)
			}

			if gotSummary != fixture.Summary {
				t.Fatalf("summary mismatch: got %+v, want %+v", gotSummary, fixture.Summary)
			}

			wantOutput := strings.TrimSpace(fixture.Result.Output)
			if gotOutput != wantOutput {
				t.Fatalf("output mismatch:\n got: %q\nwant: %q", gotOutput, wantOutput)
			}

			if fixture.Result.ExitCode != 0 {
				t.Fatalf("fixture exit_code=%d, want 0 for replay baseline", fixture.Result.ExitCode)
			}

			if !claudeFixtureHasMetaPrefix(fixture, "agent=claude") {
				t.Fatal("fixture missing agent=claude meta event")
			}
			if !claudeFixtureHasMetaContains(fixture, "command=", "--output-format stream-json") {
				t.Fatal("fixture command meta does not include --output-format stream-json")
			}
			if !claudeFixtureHasMetaContains(fixture, "command=", "--verbose") {
				t.Fatal("fixture command meta does not include --verbose")
			}
		})
	}
}

func claudeFixtureHasMetaPrefix(fixture claudeFixture, prefix string) bool {
	for _, ev := range fixture.Events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, prefix) {
			return true
		}
	}
	return false
}

func claudeFixtureHasMetaContains(fixture claudeFixture, prefix, contains string) bool {
	for _, ev := range fixture.Events {
		if ev.Type != "meta" || !strings.HasPrefix(ev.Data, prefix) {
			continue
		}
		if strings.Contains(ev.Data, contains) {
			return true
		}
	}
	return false
}
