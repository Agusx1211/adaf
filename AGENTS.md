# AGENTS.md

Instructions and practices for AI agents working on this codebase.

## Project Overview

ADAF (Autonomous Developer Agent Flow) is a Go CLI that orchestrates AI coding agents (Claude, Codex, Gemini, Vibe, OpenCode, and arbitrary generic CLIs). It wraps these tools as child processes, parses their streaming output, records all I/O, and manages multi-agent collaboration through structured session handoffs, worktree isolation, and a persistent project store.

## Architecture

```
cmd/adaf/              Entry point
internal/
  buildinfo/           Build metadata and version helpers
  agent/               Agent interface + per-tool implementations (claude, codex, vibe, gemini, opencode, generic)
  agentmeta/           Built-in metadata catalog (models, capabilities, reasoning levels)
  cli/                 Cobra commands (~25 subcommands)
  config/              Global user config (~/.adaf/config.json)
  debug/               Runtime debug tools and diagnostics
  detect/              PATH scanning, version probing, dynamic model discovery
  eventq/              Local event queue and dispatch
  loop/                Single-agent loop controller (turn management, recording, callbacks)
  looprun/             Multi-step loop runtime
  orchestrator/        Sub-agent spawning with worktree isolation and concurrency limits
  hexid/               Stable identifiers for sessions and work items
  pushover/            Pushover notification integration
  stats/               Statistics extraction from recordings
  prompt/              Context-aware prompt building
  recording/           Session I/O recording (NDJSON)
  session/             Detachable session management
  store/               File-based project store (.adaf/)
  stream/              NDJSON stream parsing and terminal display
  webserver/           Internal webserver helpers
  worktree/            Git worktree lifecycle
pkg/protocol/          Agent instruction protocol (system prompt generation)
```

Key data flow: **CLI command -> Loop -> Agent.Run() -> child process (exec) -> stream parser -> recorder/event sink**

All agent integrations are CLI wrappers via `os/exec`. There are zero Go library dependencies on any agent SDK.

## Reference Repositories

The `./references/` directory contains cloned source code of the external agent CLIs that this project integrates with. **Always consult these when working on agent integrations** instead of guessing CLI flags, output formats, stream protocols, or model identifiers.

### Setup

```bash
mkdir -p references
git clone https://github.com/anthropics/claude-code references/claude-code
git clone https://github.com/openai/codex references/codex
git clone https://github.com/mistralai/mistral-vibe references/vibe
git clone https://github.com/google-gemini/gemini-cli references/gemini-cli
```

### When to consult references

| Area | Reference | What to verify |
|------|-----------|----------------|
| `internal/agent/claude.go` | `references/claude-code/` | CLI flags, `--output-format stream-json` schema, model IDs |
| `internal/agent/codex.go` | `references/codex/` | `exec` subcommand, `--json`, `--dangerously-bypass-approvals-and-sandbox`, model slugs |
| `internal/agent/vibe.go` | `references/vibe/` | `-p` flag, config.toml format, model aliases |
| `internal/agent/gemini.go` | `references/gemini-cli/` | `-p` flag, `--output-format stream-json`, `-y` auto-approve |
| `internal/detect/detect.go` | All | Version flags, install paths, config file locations, model discovery |
| `internal/stream/` | `references/claude-code/`, `references/gemini-cli/` | NDJSON event types, content block schemas |
| `internal/agentmeta/catalog.go` | All | Supported models, capabilities, defaults |

**Do not guess.** If a CLI flag, output format, or model name is in question, look it up in the reference source. If something isn't working as expected, `git -C references/<repo> pull` and verify against the latest.

The `references/` directory is git-ignored. Never commit files from it.

## Go Conventions

### Style

- Standard `gofmt -s` formatting. Run `make fmt` before committing.
- No external assertion libraries. Use stdlib `testing` only.
- Error handling follows Go idioms: return `(value, error)`, check with `if err != nil`.
- Use `errors.As` / `errors.Is` for error inspection (see `codex.go` for the pattern).
- Best-effort on non-critical paths (e.g. recording flush failures are logged, not fatal).

### Naming

- Constructors: `New<Type>()` (e.g. `NewClaudeAgent()`, `NewDisplay()`)
- Interface: `Agent` in `agent.go` defines the contract. All agent types implement it.
- Agent names are lowercase canonical strings: `"claude"`, `"codex"`, `"vibe"`, `"gemini"`, `"opencode"`, `"generic"`.
- Config structs: `*Config`, data records: `*Record`, events: `*Event`.

### Package Boundaries

- Everything under `internal/` is private. The only public package is `pkg/protocol/`.
- Each package has a single responsibility. Don't cross-import between peer packages when avoidable.
- The `agent` package owns the `Agent` interface, `Config`, `Result`, and the global registry.
- The `stream` package handles parsing and display independently of which agent produced the output.

## Agent Integration Patterns

When adding or modifying an agent integration, follow these patterns from the existing implementations:

### Agent Interface

Every agent implements:
```go
type Agent interface {
    Name() string
    Run(ctx context.Context, cfg Config, recorder *recording.Recorder) (*Result, error)
}
```

### Run() Implementation Checklist

1. Resolve binary: `cfg.Command` with fallback to the canonical name.
2. Build args: start from `cfg.Args`, then append agent-specific flags.
3. Set up `exec.CommandContext` with `cfg.WorkDir` and environment overlay.
4. For agents that spawn child processes (codex, gemini): set `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` and a `cmd.Cancel` that kills the process group.
5. Record metadata via `recorder.RecordMeta()`.
6. Capture output: either NDJSON stream parsing (claude, gemini) or simple buffer capture (codex, vibe, generic).
7. Support `cfg.EventSink` when the agent produces stream events.
8. Return `*Result` with exit code, duration, captured output, and stderr.

### Stream vs. Buffer Agents

- **Stream agents** (claude, gemini): pipe stdout, parse NDJSON via `stream.Parse()` / `stream.ParseGemini()`, forward events to `EventSink` or `Display`.
- **Buffer agents** (codex, vibe, opencode, generic): capture stdout/stderr into `bytes.Buffer` via `io.MultiWriter`.

Don't mix these patterns. If an agent doesn't produce structured streaming output, use the buffer approach.

### Adding a New Agent

1. Create `internal/agent/<name>.go` implementing the `Agent` interface.
2. Register it in `internal/agent/registry.go` in `DefaultRegistry()`.
3. Add metadata to `internal/agentmeta/catalog.go` (binary name, default model, capabilities).
4. If the tool has dynamic model discovery, add a `probe<Name>Models()` function in `internal/detect/detect.go`.
5. If the tool produces NDJSON, add a parser in `internal/stream/`.
6. Add the tool's binary to `knownBinaryCandidates()` in `detect.go`.
7. Write unit tests and an integration test (with `//go:build integration` tag).

## Testing

### Unit Tests

- Use table-driven subtests:
  ```go
  tests := []struct {
      name string
      input string
      want  string
  }{ ... }
  for _, tt := range tests {
      t.Run(tt.name, func(t *testing.T) { ... })
  }
  ```
- Use `t.TempDir()` for filesystem tests, `t.Helper()` for setup functions.
- Assertions with plain `if` + `t.Errorf` / `t.Fatalf`. No testify, no gomock.

### Integration Tests

- Gated with `//go:build integration`. Not run in CI by default.
- Skip gracefully if the agent binary is missing: `t.Skipf("binary not found")`.
- Use generous timeouts (120s) since real agent CLIs are slow.
- Test real prompts, verify output contains expected markers.

### Running Tests

```bash
make test                          # unit tests only
go test -tags=integration ./...    # include integration tests (requires agent CLIs installed)
make lint                          # golangci-lint
make all                           # tidy + fmt + build + test
```

## Build

```bash
make build     # -> bin/adaf (version injected via ldflags)
make install   # -> $GOPATH/bin/adaf
```

Version is derived from `git describe --tags --always --dirty`.

## Common Pitfalls

- **Process cleanup**: Node.js-based CLIs (claude, gemini, codex) spawn child processes. Without `Setpgid` + process group kill, orphan processes will hold pipes open and hang the parent. Always use the process group pattern for new agents backed by Node.js tools.
- **Model discovery is fragile**: Each agent stores models in a different format and location (JSON cache, TOML config, JS bundle, etc.). The `detect` package uses regexes and file parsing that can break with upstream updates. When something stops working, check the reference repo for format changes.
- **Stream event types change**: Claude and Gemini can add new NDJSON event types across versions. The parsers should silently ignore unknown types rather than failing.
- **Context cancellation**: All `Agent.Run()` calls must respect `ctx`. Never block indefinitely. The loop controller and orchestrator rely on context cancellation for graceful shutdown.
- **Recorder is not optional**: Even if you don't care about recordings, `Agent.Run()` expects a non-nil `*recording.Recorder`. Tests create one with a temp directory.
- **Registry is global and mutex-protected**: Don't hold references to the registry map across goroutines. Use `agent.Get()` / `agent.All()` which copy under lock.
