package buildinfo

import (
	"runtime/debug"
	"strings"
	"time"
)

// Linker-overridable build metadata.
var (
	Version    = "0.1.0"
	CommitHash = ""
	BuildDate  = ""
)

// Info is normalized build metadata for display.
type Info struct {
	Version    string
	CommitHash string
	BuildDate  string
}

// Current returns build metadata from linker overrides, with runtime build
// settings as fallback when available.
func Current() Info {
	info := Info{
		Version:    strings.TrimSpace(Version),
		CommitHash: strings.TrimSpace(CommitHash),
		BuildDate:  strings.TrimSpace(BuildDate),
	}

	var vcsRevision string
	var vcsTime string
	vcsDirty := false

	if bi, ok := debug.ReadBuildInfo(); ok {
		if (info.Version == "" || info.Version == "0.1.0") && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}

		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				vcsRevision = strings.TrimSpace(s.Value)
			case "vcs.time":
				vcsTime = strings.TrimSpace(s.Value)
			case "vcs.modified":
				vcsDirty = strings.EqualFold(strings.TrimSpace(s.Value), "true")
			}
		}
	}

	if info.CommitHash == "" {
		info.CommitHash = vcsRevision
		if info.CommitHash != "" && vcsDirty && !strings.HasSuffix(info.CommitHash, "-dirty") {
			info.CommitHash += "-dirty"
		}
	}

	if info.BuildDate == "" {
		info.BuildDate = vcsTime
	}
	if parsed, err := time.Parse(time.RFC3339, info.BuildDate); err == nil {
		info.BuildDate = parsed.UTC().Format("2006-01-02 15:04:05 UTC")
	}

	if info.Version == "" {
		info.Version = "unknown"
	}
	if info.CommitHash == "" {
		info.CommitHash = "unknown"
	}
	if info.BuildDate == "" {
		info.BuildDate = "unknown"
	}
	return info
}
