# adaf - Autonomous Developer Agent Flow

An orchestrator for AI coding agents. adaf manages plans, issues, docs, session logs, decisions, and deep session recordings **outside** the target repository, so multiple AI agents (claude, codex, vibe, etc.) can collaborate on a codebase via structured relay handoffs.

## Install

```bash
go install github.com/agusx1211/adaf/cmd/adaf@latest
```

Or build from source:

```bash
git clone https://github.com/agusx1211/adaf.git
cd adaf
make install
```

## Quick Start

```bash
# Initialize adaf on your project
cd /path/to/your/repo
adaf init --name "my-project"

# Set a plan
cat plan.json | adaf plan set

# Run an agent
adaf run --agent claude --max-turns 1

# Check status
adaf status

# Launch interactive TUI
adaf tui
```

## Commands

| Command | Description |
|---------|-------------|
| `adaf init` | Initialize a new project (creates `.adaf/` directory) |
| `adaf run` | Run an agent loop (`--agent claude/codex/vibe/generic`) |
| `adaf status` | Show project overview (plan progress, issues, sessions) |
| `adaf plan show` | Display the current plan with phases |
| `adaf plan set` | Set plan from file or stdin (JSON) |
| `adaf plan phase-status <id> <status>` | Update a phase's status |
| `adaf issue list` | List issues (with `--status` filter) |
| `adaf issue create` | Create an issue |
| `adaf issue update <id>` | Update an issue |
| `adaf log list` | List session logs |
| `adaf log latest` | Show most recent session log |
| `adaf log create` | Write a session log entry |
| `adaf doc list/show/create/update` | Manage project documents |
| `adaf decision list/show/create` | Manage architectural decisions |
| `adaf session list/show` | View session recordings |
| `adaf tui` | Launch interactive terminal dashboard |

## How It Works

adaf stores all project management state in a `.adaf/` directory:

```
.adaf/
  project.json        # Project config
  plan.json           # Plan with phases
  issues/             # Issue tracker (JSON per issue)
  logs/               # Session logs (JSON per session)
  decisions/          # Architectural decision records
  docs/               # Project documents
  recordings/         # Deep session recordings (stdin/stdout/stderr)
```

This keeps orchestration state **separate from your codebase** -- no more plans, logs, and issues cluttering your repo.

## Agent Support

adaf wraps existing AI CLI tools. It does not reimplement them.

| Agent | CLI | How adaf invokes it |
|-------|-----|---------------------|
| claude | `claude` | `claude -p <prompt> [--model ...] [--dangerously-skip-permissions]` |
| codex | `codex` | `codex exec <prompt> [--model ...] [--full-auto]` |
| vibe | `vibe` | `vibe <prompt>` |
| generic | any | Custom command with prompt piped to stdin |

All agent I/O (stdin, stdout, stderr) is recorded to `.adaf/recordings/` for future analysis.

## Agent CLI Interface

Agents running inside adaf can call back into `adaf` to read/write project state:

```bash
# Orient
adaf status
adaf log latest
adaf plan show

# Track work
adaf issue create --title "Bug found" --priority high
adaf issue update 3 --status resolved

# Log session
adaf log create --agent claude --objective "Fix auth" --built "..." --next "..."

# Record decisions
adaf decision create --title "Use JWT" --context "..." --decision "..." --rationale "..."
```

## Interactive TUI

`adaf tui` launches a full-screen dashboard with 6 views:

- **Dashboard** -- project overview with plan progress bar, issue counts, recent sessions
- **Plan** -- phase list with status indicators, detail panel
- **Issues** -- issue table with status/priority filtering
- **Logs** -- session log browser
- **Sessions** -- recording viewer with event timeline
- **Docs** -- document browser

Navigate with `tab`/`1-6`, browse with `j/k`, expand with `enter`, back with `esc`.

## License

MIT
