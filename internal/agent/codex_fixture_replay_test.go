package agent

import (
	"strings"
	"testing"
)

func TestCodexFixtureReplay(t *testing.T) {
	fixtures := mustReadCodexFixtures(t)
	if len(fixtures) == 0 {
		t.Skipf("no Codex fixtures found in %s; run %s=1 go test -tags=integration ./internal/agent -run TestRecordCodexFixture -v", codexFixtureDir, codexFixtureRecordEnv)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run("codex-"+fixture.Version, func(t *testing.T) {
			if fixture.SchemaVersion != codexFixtureSchemaVersionV1 {
				t.Fatalf("unsupported schema_version=%d", fixture.SchemaVersion)
			}
			if fixture.Provider != "codex" {
				t.Fatalf("fixture provider=%q, want %q", fixture.Provider, "codex")
			}

			ndjson := codexFixtureNDJSON(fixture)
			if strings.TrimSpace(ndjson) == "" {
				t.Fatal("fixture has no claude_stream events")
			}

			gotSummary, gotOutput, err := summarizeCodexFixtureStream(ndjson)
			if err != nil {
				t.Fatalf("summarizeCodexFixtureStream() error: %v", err)
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

			if !codexFixtureHasMetaPrefix(fixture, "agent=codex") {
				t.Fatal("fixture missing agent=codex meta event")
			}
			if !codexFixtureHasMetaContains(fixture, "command=", "exec") {
				t.Fatal("fixture command meta does not include exec")
			}
			if !codexFixtureHasMetaContains(fixture, "command=", "--json") {
				t.Fatal("fixture command meta does not include --json")
			}
		})
	}
}

func codexFixtureHasMetaPrefix(fixture codexFixture, prefix string) bool {
	for _, ev := range fixture.Events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, prefix) {
			return true
		}
	}
	return false
}

func codexFixtureHasMetaContains(fixture codexFixture, prefix, contains string) bool {
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
