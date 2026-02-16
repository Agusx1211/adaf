package agent

import (
	"strings"
	"testing"
)

func TestGeminiFixtureReplay(t *testing.T) {
	fixtures := mustReadGeminiFixtures(t)
	if len(fixtures) == 0 {
		t.Skipf("no Gemini fixtures found in %s; run %s=1 go test -tags=integration ./internal/agent -run TestRecordGeminiFixture -v", geminiFixtureDir, geminiFixtureRecordEnv)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run("gemini-"+fixture.Version, func(t *testing.T) {
			if fixture.SchemaVersion != geminiFixtureSchemaVersionV1 {
				t.Fatalf("unsupported schema_version=%d", fixture.SchemaVersion)
			}
			if fixture.Provider != "gemini" {
				t.Fatalf("fixture provider=%q, want %q", fixture.Provider, "gemini")
			}

			ndjson := geminiFixtureNDJSON(fixture)
			if strings.TrimSpace(ndjson) == "" {
				t.Fatal("fixture has no claude_stream events")
			}

			gotSummary, gotOutput, err := summarizeGeminiFixtureStream(ndjson)
			if err != nil {
				t.Fatalf("summarizeGeminiFixtureStream() error: %v", err)
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

			if !geminiFixtureHasMetaPrefix(fixture, "agent=gemini") {
				t.Fatal("fixture missing agent=gemini meta event")
			}
			if !geminiFixtureHasMetaContains(fixture, "command=", "--output-format stream-json") {
				t.Fatal("fixture command meta does not include --output-format stream-json")
			}
		})
	}
}

func geminiFixtureHasMetaPrefix(fixture geminiFixture, prefix string) bool {
	for _, ev := range fixture.Events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, prefix) {
			return true
		}
	}
	return false
}

func geminiFixtureHasMetaContains(fixture geminiFixture, prefix, contains string) bool {
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
