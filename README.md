<p align="center">
  <pre align="center">
     _       _        __
    / \   __| | __ _ / _|
   / _ \ / _` |/ _` | |_
  / ___ \ (_| | (_| |  _|
 /_/   \_\__,_|\__,_|_|
  </pre>
</p>

<h3 align="center">Autonomous Developer Agent Flow</h3>

<p align="center">
  Orchestrate AI coding agents to build, plan, and maintain software projects.
</p>

<p align="center">
  <a href="https://github.com/agusx1211/adaf/actions/workflows/ci.yml"><img src="https://github.com/agusx1211/adaf/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/agusx1211/adaf/actions/workflows/release.yml"><img src="https://github.com/agusx1211/adaf/actions/workflows/release.yml/badge.svg" alt="Release"></a>
  <a href="https://github.com/agusx1211/adaf/releases"><img src="https://img.shields.io/github/v/release/agusx1211/adaf?include_prereleases" alt="Release"></a>
  <a href="https://pkg.go.dev/github.com/agusx1211/adaf"><img src="https://pkg.go.dev/badge/github.com/agusx1211/adaf.svg" alt="Go Reference"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

---

**adaf** is a meta-orchestrator for AI coding agents. It manages plans, issues, docs, session logs, and deep session recordings **outside** the target repository, so multiple AI agents can collaborate on a codebase via structured relay handoffs.

Think of it as project management infrastructure purpose-built for AI agents -- plans with phases, an issue tracker, session logs for handoffs, sub-agent spawning with worktree isolation, looping workflows, detachable sessions, and full I/O recording. All stored in `.adaf/`, never cluttering your repo.

## Why adaf?

When you run AI coding agents (Claude, Codex, Gemini, etc.) on a project, each session starts from scratch. There's no shared memory of what was tried, what decisions were made, or what the current plan is. **adaf solves this:**

- **Shared project state** -- Plans, issues, docs, and session logs persist across agent sessions
- **Relay handoffs** -- Each agent logs what it did, so the next agent picks up where it left off
- **Multi-agent orchestration** -- Loops chain agent profiles together; spawns delegate subtasks to child agents in isolated worktrees
- **Full recording** -- Every agent interaction (stdin/stdout/stderr) is captured for analysis
- **Agent-agnostic** -- Wraps existing CLI tools without replacing them
- **Stays out of your repo** -- All state lives in `.adaf/`, not in your source code

## Supported Agents

| Agent | CLI Tool | Invocation |
|-------|----------|------------|
| **claude** | `claude` | `claude -p <prompt> --output-format stream-json --verbose` |
| **codex** | `codex` | `codex exec <prompt> --dangerously-bypass-approvals-and-sandbox --json` |
| **gemini** | `gemini` | `gemini <prompt> -y` |
| **vibe** | `vibe` | `vibe <prompt>` (stdin) |
| **opencode** | `opencode` | `opencode` |
| **generic** | any | Custom command with prompt piped to stdin |

adaf auto-detects installed agents via `adaf config agents detect` and supports per-agent model overrides, reasoning levels, and health checks.

## Installation

### From source (recommended)

```bash
go install github.com/agusx1211/adaf/cmd/adaf@latest
```

### Build from source

```bash
git clone https://github.com/agusx1211/adaf.git
cd adaf
make install
```

### From releases

Download pre-built binaries from the [releases page](https://github.com/agusx1211/adaf/releases).

## Quick Start

```bash
# 1. Initialize adaf on your project
cd /path/to/your/repo
adaf init --name "my-project"

# 2. Set a project plan
adaf plan set plan.json

# 3. Run an agent session
adaf run --agent claude --max-turns 1

# 4. Check project status
adaf status

# 5. Check project status
adaf
```

## Commands

### Core

| Command | Aliases | Description |
|---------|---------|-------------|
| `adaf` | | Show a concise status summary (or JSON status with `--json`) |
| `adaf init` | `setup` | Initialize a new project (creates `.adaf/` directory) |
| `adaf run` | `exec` | Run an agent against the project |
| `adaf status` | `st`, `info` | Show comprehensive project status |
| `adaf attach <id>` | `connect` | Reattach to a running detached session |
| `adaf sessions` | | List all active/completed sessions |

### Project Management

| Command | Aliases | Description |
|---------|---------|-------------|
| `adaf plan [show]` | `plans` | Display the current plan with phases |
| `adaf plan set [file]` | `load`, `import` | Set plan from JSON file or stdin |
| `adaf plan phase-status <id> <status>` | | Update a phase's status |
| `adaf issue list` | `ls` | List issues (with `--status` filter) |
| `adaf issue create` | `new`, `add` | Create a new issue |
| `adaf issue show <id>` | `get`, `view` | Show issue details |
| `adaf issue update <id>` | `edit` | Update issue status/priority/labels |
| `adaf log list` | `ls` | List session logs |
| `adaf log latest` | `last` | Show the most recent session log |
| `adaf log create` | `new` | Create a session log entry |
| `adaf doc list` | `ls` | List project documents |
| `adaf doc create` | `new` | Create a document (from file or inline) |
| `adaf doc show <id>` | `get` | Display a document |

### Configuration

| Command | Aliases | Description |
|---------|---------|-------------|
| `adaf config agents [list]` | `agent` | List detected agent tools |
| `adaf config agents detect` | `scan`, `refresh` | Scan PATH for agent tools |
| `adaf config agents set-model <agent> <model>` | | Set default model for an agent |
| `adaf config agents test <agent>` | `health-check` | Run a health-check prompt |
| `adaf config pushover setup` | | Configure Pushover notification credentials |
| `adaf config pushover test` | | Send a test Pushover notification |
| `adaf config pushover status` | | Show Pushover configuration status |

### Orchestration

| Command | Aliases | Description |
|---------|---------|-------------|
| `adaf loop list` | `ls` | List defined loop templates |
| `adaf loop start <name>` | `run` | Start a loop (cyclic agent workflow) |
| `adaf loop stop` | `halt` | Signal the current loop to stop |
| `adaf loop status` | `info` | Show active loop run status |
| `adaf loop message <text>` | `msg` | Post a message to subsequent loop steps |
| `adaf loop notify <title> <msg>` | | Send a Pushover notification from a loop step |
| `adaf spawn` | `fork` | Spawn a sub-agent in an isolated worktree |
| `adaf spawn-status` | | Show status of spawned sub-agents |
| `adaf spawn-wait` | | Wait for spawned sub-agents to complete |
| `adaf spawn-diff` | | Show diff of a spawn's changes |
| `adaf spawn-merge` | | Merge a spawn's changes into current branch |
| `adaf spawn-reject` | | Reject a spawn's changes and clean up |
| `adaf spawn-watch` | | Watch spawn output in real-time |
| `adaf tree` | `hierarchy` | Show agent hierarchy tree |

### Communication

| Command | Description |
|---------|-------------|
| `adaf parent-ask <question>` | Ask parent agent a question (blocks until answered) |
| `adaf spawn-reply <answer>` | Reply to a child agent's question |
| `adaf spawn-message <msg>` | Send an async message to a child agent |
| `adaf spawn-read-messages` | Read unread messages from parent |

### Analysis

| Command | Description |
|---------|-------------|
| `adaf stats` | Show profile and loop statistics |
| `adaf stats profile <name>` | Detailed stats for a profile |
| `adaf stats loop <name>` | Detailed stats for a loop |
| `adaf stats migrate` | Retroactively compute stats from recordings |
| `adaf stats profile <name> --format markdown` | Export profile history as markdown for LLM analysis |
| `adaf stats loop <name> --format markdown` | Export loop history as markdown for LLM analysis |
### Utilities

| Command | Aliases | Description |
|---------|---------|-------------|
| `adaf cleanup --list` | | List active adaf-managed worktrees |
| `adaf cleanup --max-age 0` | | Remove all adaf worktrees (crash recovery) |

## How It Works

### Project State

adaf stores all orchestration state in a `.adaf/` directory at the root of your project:

```
.adaf/
  project.json        # Project metadata (name, repo path, agent config)
  plans/              # Plans (one JSON file per plan)
  issues/             # Issue tracker (one JSON file per issue)
  turns/              # Session logs (one JSON per turn)
  docs/               # Project documents
  records/            # Deep session recordings (stdin/stdout/stderr)
  stats/              # Profile and loop statistics
  spawns/             # Sub-agent orchestration state
  messages/           # Parent/child spawn message channels
  loopruns/           # Loop execution state
```

This keeps orchestration state **separate from your codebase**. The `.adaf/.gitignore` is configured to keep ephemeral data (recordings, logs, agents cache) out of version control, while plans, issues, and documents can be committed.

### Agent Prompt Building

When you run `adaf run`, adaf automatically builds a context-rich prompt from the project state:
- Current plan with phase statuses
- Open issues
- Latest session log (what the previous agent did)
- Available agent tools and commands

This gives each agent a full picture of the project without manual copy-pasting.

### Session Recordings

Every agent interaction is recorded to `.adaf/records/<session-id>/`:
- `events.jsonl` -- NDJSON stream of timestamped events (stdin, stdout, stderr, parsed stream events)
- Metadata (agent, model, start/end time, exit code)

Use `adaf stats migrate` to extract cost/token/tool usage metrics from recordings.

## Multi-Agent Workflows

### Loops

Loops define cyclic workflows where multiple agent profiles take turns working on the project:

```json
{
  "loops": [
    {
      "name": "dev-cycle",
      "steps": [
        {
          "profile": "builder",
          "role": "developer",
          "turns": 3,
          "instructions": "Implement the next planned feature",
          "delegation": {
            "profiles": [
              { "name": "tester" }
            ]
          }
        },
        {
          "profile": "reviewer",
          "role": "lead-developer",
          "turns": 1,
          "instructions": "Review and fix issues",
          "can_stop": true
        },
        {
          "profile": "tester",
          "role": "developer",
          "turns": 1,
          "instructions": "Run tests and file issues"
        }
      ]
    }
  ]
}
```

```bash
adaf loop start dev-cycle
```

Each step runs the specified profile, and steps can communicate via `adaf loop message`, send notifications via `adaf loop notify`, or signal the loop to stop via `adaf loop stop`.

### Sub-Agent Spawning

Agents can delegate subtasks to child agents that work in isolated git worktrees:

```bash
# From inside an agent session:
adaf spawn --profile builder --role developer --task "Write unit tests for auth.go"
adaf spawn --profile builder --role developer --task "Refactor database layer" --wait

# Monitor spawns
adaf tree
adaf spawn-status
adaf spawn-watch --spawn-id 3

# Review and merge
adaf spawn-diff --spawn-id 3
adaf spawn-merge --spawn-id 3
```

Child agents run in their own git branches. Results can be reviewed, merged, or rejected.

### Agent Profiles

Profiles define reusable agent/model characteristics:

```json
{
  "profiles": [
    {
      "name": "reviewer",
      "agent": "claude",
      "model": "claude-opus-4-6",
      "intelligence": 9,
      "description": "Senior engineer for complex tasks and code review"
    },
    {
      "name": "builder",
      "agent": "claude",
      "model": "claude-sonnet-4-5-20250929",
      "reasoning_level": "high",
      "intelligence": 7,
      "description": "Implementation-focused engineer"
    }
  ]
}
```

Roles and spawn permissions are configured per loop step (`loops[].steps[]`), not per profile.

## Configuration

### Global Config (`~/.adaf/config.json`)

```json
{
  "agents": {
    "claude": { "model_override": "claude-opus-4-6" }
  },
  "profiles": [ ... ],
  "loops": [ ... ],
  "pushover": {
    "user_key": "...",
    "app_token": "..."
  }
}
```

### Project Config (`.adaf/project.json`)

Created by `adaf init`. Contains project name, repo path, and project-level agent configuration overrides.

### Config Priority

1. CLI flags (highest)
2. Agent detection cache (`~/.adaf/agents.json`)
3. Global user-level config (`~/.adaf/config.json`)

## Default command behavior

Running `adaf` with no arguments prints a concise project status summary (or JSON when `--json` is provided).

## Detachable Sessions

Run agents as background sessions (like tmux) that survive terminal disconnects:

```bash
# Start a detached session
adaf run --agent claude -s

# List sessions
adaf sessions

# Reattach to a running session
adaf attach <session-id>

# Inside attached session:
#   Ctrl+D  -- detach (agent keeps running)
#   Ctrl+C  -- stop agent and detach
```

## Notifications

adaf integrates with [Pushover](https://pushover.net) for mobile/desktop push notifications from loop steps:

```bash
# Set up credentials
adaf config pushover setup

# Send from a loop step
adaf loop notify "Build Complete" "All tests passing" --priority 1
```

## Agent CLI Interface

Agents running inside adaf can call back into the CLI to read/write project state:

```bash
# Orient
adaf status                                    # Project overview
adaf log latest                                # What the last agent did
adaf plan show                                 # Current plan

# Track work
adaf issue create --title "Bug found" --priority high
adaf issue update 3 --status resolved

# Log session (handoff to next agent)
adaf log create --agent claude \
  --objective "Fix auth module" \
  --built "JWT implementation with refresh tokens" \
  --decisions "Kept RS256 + refresh flow for consistency with existing services" \
  --next "Add rate limiting to auth endpoints"

# Orchestrate
adaf spawn --profile builder --role developer --task "Write tests for auth.go"
adaf tree                                      # View spawn hierarchy
```

## Project Structure

```
cmd/adaf/              Entry point
internal/
  agent/               Agent implementations (claude, codex, vibe, opencode, gemini, generic)
  agentmeta/           Agent metadata catalog
  cli/                 Cobra CLI commands (25+ commands)
  config/              Global configuration (~/.adaf/config.json)
  detect/              Agent auto-detection (PATH scanning)
  loop/                Single-agent loop controller
  looprun/             Multi-step loop runtime
  orchestrator/        Sub-agent orchestration (spawn/merge/reject)
  project/             Project management
  prompt/              Context-aware prompt building
  pushover/            Pushover notification client
  recording/           Session I/O recording and playback
  session/             Detachable session management (daemon/client)
  eventq/              Local event queue and dispatch
  stats/               Statistics extraction from recordings
  store/               File-based project store (.adaf/ directory)
  stream/              Agent output stream parsing (NDJSON)
  webserver/           Internal webserver helpers
  worktree/            Git worktree management for sub-agents
pkg/protocol/          Agent protocol documentation
```

## Development

```bash
# Build
make build

# Run tests
make test

# Format code
make fmt

# Lint (requires golangci-lint)
make lint

# All checks
make all
```

## License

MIT
