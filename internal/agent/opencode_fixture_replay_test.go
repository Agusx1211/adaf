package agent

import (
	"strings"
	"testing"
)

func TestOpencodeFixtureReplay(t *testing.T) {
	fixtures := mustReadOpencodeFixtures(t)
	if len(fixtures) == 0 {
		t.Skipf("no OpenCode fixtures found in %s; run %s=1 go test -tags=integration ./internal/agent -run TestRecordOpencodeFixture -v", opencodeFixtureDir, opencodeFixtureRecordEnv)
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run("opencode-"+fixture.Version, func(t *testing.T) {
			if fixture.SchemaVersion != opencodeFixtureSchemaVersionV1 {
				t.Fatalf("unsupported schema_version=%d", fixture.SchemaVersion)
			}
			if fixture.Provider != "opencode" {
				t.Fatalf("fixture provider=%q, want %q", fixture.Provider, "opencode")
			}

			ndjson := opencodeFixtureNDJSON(fixture)
			if strings.TrimSpace(ndjson) == "" {
				t.Fatal("fixture has no claude_stream events")
			}

			gotSummary, gotOutput, err := summarizeOpencodeFixtureStream(ndjson)
			if err != nil {
				t.Fatalf("summarizeOpencodeFixtureStream() error: %v", err)
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

			if !opencodeFixtureHasMetaPrefix(fixture, "agent=opencode") {
				t.Fatal("fixture missing agent=opencode meta event")
			}
			if !opencodeFixtureHasMetaContains(fixture, "command=", "run") {
				t.Fatal("fixture command meta does not include run subcommand")
			}
			if !opencodeFixtureHasMetaContains(fixture, "command=", "--format json") {
				t.Fatal("fixture command meta does not include --format json")
			}
		})
	}
}

func opencodeFixtureHasMetaPrefix(fixture opencodeFixture, prefix string) bool {
	for _, ev := range fixture.Events {
		if ev.Type == "meta" && strings.HasPrefix(ev.Data, prefix) {
			return true
		}
	}
	return false
}

func opencodeFixtureHasMetaContains(fixture opencodeFixture, prefix, contains string) bool {
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
