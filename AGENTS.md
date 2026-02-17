# AGENTS.md

Instructions and practices for AI agents working on this codebase.

## Project Overview

ADAF (Autonomous Developer Agent Flow) is a Go CLI + React web UI that orchestrates AI coding agents (Claude, Codex, Gemini, Vibe, OpenCode, and arbitrary generic CLIs). It wraps these tools as child processes, parses their streaming output, records all I/O, and manages multi-agent collaboration through structured session handoffs, worktree isolation, and a persistent project store. A built-in web server with WebSocket support provides a real-time dashboard for monitoring and interacting with running sessions.

## Development Phase

This project is in early active development. There are no external users or deployments to maintain compatibility with. Storage formats, APIs, CLI flags, and data structures can change freely at any time — just wipe `.adaf/` and start fresh. Prefer the simplest, cleanest implementation over any backwards-compatibility concern. Aggressively refactor, rename, delete dead code, and restructure without hesitation. Never add migration logic, deprecation shims, or compatibility layers.

## Architecture

```
cmd/adaf/              Entry point
internal/
  agent/               Agent interface + per-tool implementations (claude, codex, vibe, gemini, opencode, generic)
  agentmeta/           Built-in metadata catalog (models, capabilities, reasoning levels)
  buildinfo/           Build metadata and version helpers
  cli/                 Cobra commands (~60 subcommands including daemon, web, sessions, stats, usage, skills)
  config/              Global user config (~/.adaf/config.json)
  debug/               Runtime debug tools and diagnostics
  detect/              PATH scanning, version probing, dynamic model discovery
  events/              Typed messages for web UI / daemon communication (WebSocket event protocol)
  eventq/              Local event queue and dispatch
  hexid/               Stable identifiers for sessions and work items
  loop/                Single-agent loop controller (turn management, recording, callbacks)
  looprun/             Multi-step loop runtime
  orchestrator/        Sub-agent spawning with worktree isolation and concurrency limits
  prompt/              Context-aware prompt building
  pushover/            Pushover notification integration
  recording/           Session I/O recording (NDJSON)
  session/             Detachable session management
  stats/               Statistics extraction from recordings
  store/               File-based project store (.adaf/)
  stream/              NDJSON stream parsing and terminal display (parsers for claude, codex, gemini, opencode, vibe)
  usage/               Provider usage tracking and rate-limit monitoring
  webserver/           Embedded web server with REST API, WebSocket, PTY, and TLS support
  worktree/            Git worktree lifecycle
web/                   React frontend (esbuild, compiled into webserver/static/)
  src/components/      UI components (feed, detail, views, project, common, layout, tree, session, loop)
e2e/                   Playwright end-to-end tests for the web UI
```

Key data flow: **CLI command -> Loop -> Agent.Run() -> child process (exec) -> stream parser -> recorder/event sink -> WebSocket -> web UI**

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
| `internal/stream/` | All | NDJSON event types, content block schemas, agent-specific stream formats |
| `internal/agentmeta/catalog.go` | All | Supported models, capabilities, defaults |

**Do not guess.** If a CLI flag, output format, or model name is in question, look it up in the reference source. If something isn't working as expected, `git -C references/<repo> pull` and verify against the latest.

The `references/` directory is git-ignored. Never commit files from it.

## Testing Philosophy

**Every feature must have a real test. Every bug fix must start with a real test.**

This project prioritizes integration tests that exercise real system behavior over unit tests with mocked data. Tests should run against the actual system — Go integration tests use fixture replay against real agent recordings, and frontend tests use Playwright against a running web server.

### Bug Fix Protocol

When a bug or issue is reported, follow this sequence strictly:

1. **Reproduce first.** Write a test (Go integration test or Playwright E2E test) that demonstrates the failure. Run it and confirm it fails in the way the bug describes.
2. **If reproduction fails,** investigate further — inspect logs, try different reproduction strategies, or ask the reporter for clarification. Do not move on to writing a fix until you have a failing test.
3. **Fix the bug** only after the failing test is in place.
4. **Verify the fix** by running the test again and confirming it passes. The previously-failing test is now a regression test that stays in the suite permanently.

This sequence is non-negotiable. A fix without a reproducing test is incomplete.

### New Feature Protocol

Every new feature — backend or frontend — must ship with tests that verify its behavior against the real system:

- **Go features**: integration test or fixture-replay test exercising the actual code path.
- **Web UI features**: Playwright E2E test interacting with the real running application.
- **Agent integrations**: fixture-based replay test using recorded real agent output.

Do not substitute mocks for system-level behavior. Unit tests are fine for pure logic (parsers, formatters, data transforms), but any feature that touches I/O, agents, the web UI, or session management must be tested end-to-end.

### Go Tests

#### Unit Tests

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

#### Integration Tests

- Gated with `//go:build integration`. Not run in CI by default.
- Skip gracefully if the agent binary is missing: `t.Skipf("binary not found")`.
- Use generous timeouts (120s) since real agent CLIs are slow.
- Test real prompts, verify output contains expected markers.

#### Fixture Replay Tests

Each agent has a fixture-based test suite that replays recorded real agent output for deterministic testing without requiring the actual agent binary:

- **Recording**: `make record-<agent>-fixture` (e.g. `make record-claude-fixture`) runs the real agent and captures its output to `internal/agent/testdata/<agent>/`.
- **Replay**: Standard `go test` replays the captured fixture data, testing parsing, event handling, and output processing without network calls.
- Files: `*_fixture_capture_test.go`, `*_fixture_helpers_test.go`, `*_fixture_replay_test.go` per agent.
- Environment variable `ADAF_RECORD_<AGENT>_FIXTURE=1` toggles between capture and replay modes.

### Playwright E2E Tests

Frontend features are tested with Playwright against a real running ADAF web server.

- Location: `e2e/tests/*.spec.js` with shared helpers in `e2e/tests/helpers.js`.
- Browser: Chromium.
- The test suite has its own global setup/teardown that manages the web server lifecycle.
- Timeout: 60s per test. Tests run serially (`workers: 1`).
- Traces captured on first retry, video retained on failure.

```bash
make e2e                           # install deps, build web, run Playwright tests
make e2e-install                   # install Playwright dependencies only
make e2e-clean                     # clean e2e artifacts
```

### Running Tests

```bash
make test                          # Go unit tests
make race                          # Go unit tests with race detector
go test -tags=integration ./...    # include Go integration tests (requires agent CLIs)
make e2e                           # Playwright end-to-end tests
make lint                          # golangci-lint
make all                           # tidy + fmt + build + test + race
```

## Web Frontend

The web UI is a React application compiled with esbuild and embedded into the Go binary as static assets.

### Structure

- Source: `web/src/` (JSX components, React 19)
- Build: `web/esbuild.mjs` (production via `--prod`, dev via `--watch`)
- Output: compiled assets are written to `internal/webserver/static/` and embedded via `//go:embed static`
- Component directories: `common`, `detail`, `feed`, `layout`, `loop`, `project`, `session`, `tree`, `views`

### Build Commands

```bash
make web                           # install npm deps + production build
make web-install                   # install npm deps only
make web-watch                     # dev mode with file watching
```

### Web Server

The Go web server (`internal/webserver/`) provides:

- REST API endpoints for config, filesystem, sessions, chat, usage, and write operations.
- WebSocket handler for live streaming of agent events to the UI.
- PTY handler for embedded terminal support.
- TLS support.
- All static assets are embedded in the binary — no external file serving required at runtime.

## Build

```bash
make build     # -> bin/adaf (builds web frontend first, then Go binary with version via ldflags)
make install   # -> $GOPATH/bin/adaf
```

Version is derived from `git describe --tags --always --dirty`.

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

- Everything under `internal/` is private. There are no public Go packages.
- Each package has a single responsibility. Don't cross-import between peer packages when avoidable.
- The `agent` package owns the `Agent` interface, `Config`, `Result`, and the global registry.
- The `stream` package handles parsing and display independently of which agent produced the output (with per-agent parsers for claude, codex, gemini, opencode, vibe).
- The `events` package defines the typed message protocol between the backend and the web UI over WebSocket.

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
7. Create fixture capture/replay tests following the existing pattern (`*_fixture_capture_test.go`, `*_fixture_helpers_test.go`, `*_fixture_replay_test.go`).
8. Record an initial fixture: `make record-<name>-fixture`.
9. Write an integration test (with `//go:build integration` tag).

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to `main`:

- **Build & Test**: Go 1.25, builds web + binary, runs `make test`.
- **Lint**: `gofmt` formatting check + `go vet`.

## Common Pitfalls

- **Process cleanup**: Node.js-based CLIs (claude, gemini, codex) spawn child processes. Without `Setpgid` + process group kill, orphan processes will hold pipes open and hang the parent. Always use the process group pattern for new agents backed by Node.js tools.
- **Model discovery is fragile**: Each agent stores models in a different format and location (JSON cache, TOML config, JS bundle, etc.). The `detect` package uses regexes and file parsing that can break with upstream updates. When something stops working, check the reference repo for format changes.
- **Stream event types change**: Claude and Gemini can add new NDJSON event types across versions. The parsers should silently ignore unknown types rather than failing.
- **Context cancellation**: All `Agent.Run()` calls must respect `ctx`. Never block indefinitely. The loop controller and orchestrator rely on context cancellation for graceful shutdown.
- **Recorder is not optional**: Even if you don't care about recordings, `Agent.Run()` expects a non-nil `*recording.Recorder`. Tests create one with a temp directory.
- **Registry is global and mutex-protected**: Don't hold references to the registry map across goroutines. Use `agent.Get()` / `agent.All()` which copy under lock.
- **Web assets must be rebuilt**: Changes to `web/src/` require `make web` (or `make web-watch` during development) before they appear in the binary. The `make build` target handles this automatically.
- **Playwright tests need the web build**: `make e2e` builds the web frontend before running tests. If running Playwright manually, ensure the web assets are up to date.
