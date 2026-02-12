package buildinfo

import "testing"

func TestCurrentUsesOverrides(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, CommitHash, BuildDate
	defer func() {
		Version, CommitHash, BuildDate = oldVersion, oldCommit, oldDate
	}()

	Version = "v1.2.3"
	CommitHash = "abc1234"
	BuildDate = "2026-02-12T10:11:12Z"

	info := Current()
	if info.Version != "v1.2.3" {
		t.Fatalf("version = %q, want %q", info.Version, "v1.2.3")
	}
	if info.CommitHash != "abc1234" {
		t.Fatalf("commit hash = %q, want %q", info.CommitHash, "abc1234")
	}
	if info.BuildDate != "2026-02-12 10:11:12 UTC" {
		t.Fatalf("build date = %q, want %q", info.BuildDate, "2026-02-12 10:11:12 UTC")
	}
}

func TestCurrentPopulatesUnknowns(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, CommitHash, BuildDate
	defer func() {
		Version, CommitHash, BuildDate = oldVersion, oldCommit, oldDate
	}()

	Version = ""
	CommitHash = ""
	BuildDate = ""

	info := Current()
	if info.Version == "" {
		t.Fatal("version should not be empty")
	}
	if info.CommitHash == "" {
		t.Fatal("commit hash should not be empty")
	}
	if info.BuildDate == "" {
		t.Fatal("build date should not be empty")
	}
}
