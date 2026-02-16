package agent

import (
	"strings"
	"testing"
)

func TestVibeFixtureReplay(t *testing.T) {
	fixtures := mustReadVibeFixtures(t)
	if len(fixtures) == 0 {
		t.Skipf("no Vibe fixtures found in %s; run %s=1 go test -tags=integration ./internal/agent -run TestRecordVibeFixture -v", vibeFixtureDir, vibeFixtureRecordEnv)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run("vibe-"+fixture.Version, func(t *testing.T) {
			if fixture.SchemaVersion != vibeFixtureSchemaVersionV1 {
				t.Fatalf("unsupported schema_version=%d", fixture.SchemaVersion)
			}
			if fixture.Provider != "vibe" {
				t.Fatalf("fixture provider=%q, want %q", fixture.Provider, "vibe")
			}

			ndjson := fixtureNDJSON(fixture)
			if strings.TrimSpace(ndjson) == "" {
				t.Fatal("fixture has no claude_stream events")
			}

			gotSummary, gotOutput, err := summarizeVibeFixtureStream(ndjson)
			if err != nil {
				t.Fatalf("summarizeVibeFixtureStream() error: %v", err)
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

			if !fixtureHasMetaPrefix(fixture, "agent=vibe") {
				t.Fatal("fixture missing agent=vibe meta event")
			}
			if !fixtureHasMetaContains(fixture, "command=", "--output streaming") {
				t.Fatal("fixture command meta does not include --output streaming")
			}
		})
	}
}

func fixtureHasMetaPrefix(fixture vibeFixture, prefix string) bool {
	for _, ev := range fixture.Events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, prefix) {
			return true
		}
	}
	return false
}

func fixtureHasMetaContains(fixture vibeFixture, prefix, contains string) bool {
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
