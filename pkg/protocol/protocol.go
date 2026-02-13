// Package protocol defines the interface contract between adaf and AI agents.
//
// Agents running inside adaf can interact with the project by calling the adaf CLI.
// This package documents the expected commands and their formats so that agents
// can be instructed (via system prompts) on how to use adaf's project management.
//
// Example agent system prompt snippet:
//
//	You have access to the `adaf` CLI for project management:
//	  adaf status                          - Show project status
//	  adaf plan show                       - Show the current plan
//	  adaf issue list                      - List open issues
//	  adaf issue create --title "..." ...  - Create an issue
//	  adaf log latest                      - Read the latest session log
//	  adaf log create --objective "..."    - Write your session log
//	  adaf decision list                   - List architectural decisions
//	  adaf doc show <id>                   - Read a project document
package protocol

// AgentInstructions returns a system prompt fragment that can be injected into
// an agent's context to teach it how to use adaf's CLI for project management.
func AgentInstructions(projectName string) string {
	return `You are working on the project "` + projectName + `" managed by adaf.

## Project Management CLI

You have access to the ` + "`adaf`" + ` CLI for managing project state. Use it to:

### Status & Orientation
- ` + "`adaf status`" + ` — Overview of project (plan progress, open issues, recent sessions)
- ` + "`adaf plan show`" + ` — View the current plan with all phases and their status
- ` + "`adaf log latest`" + ` — Read the most recent session log to understand where things stand
- ` + "`adaf log list`" + ` — List all session logs (shows ID, date, agent, objective)
- ` + "`adaf log show <id>`" + ` — Read a specific session log
- ` + "`adaf log search --query \"keyword\"`" + ` — Search session logs by keyword (across all fields)

### Issues
- ` + "`adaf issue list`" + ` — List all open issues
- ` + "`adaf issue list --status all`" + ` — List all issues including resolved
- ` + "`adaf issue create --title \"...\" --description \"...\" --priority high`" + ` — Report a new issue
- ` + "`adaf issue update <id> --status resolved`" + ` — Mark an issue as resolved

### Session Logging (REQUIRED at the end of every session)

Write a detailed session log before finishing. This is the primary handoff mechanism — the next agent relies on your log to pick up where you left off.

` + "```" + `
adaf log create --agent <your-name> \
  --objective "What you set out to do" \
  --built "What you actually built/changed" \
  --state "Current state of the codebase" \
  --next "Specific next steps for the next agent" \
  --issues "Known issues or TODOs left behind" \
  --decisions "Key decisions you made and why" \
  --challenges "Difficulties encountered" \
  --build-state "go build: OK, go test: 2 failures in pkg/foo"
` + "```" + `

**Quality guidelines:**
- Be specific: reference exact file paths, function names, and test names
- Always include build state: does ` + "`go build`" + ` succeed? Do tests pass? Which ones fail?
- Next steps should be specific enough for a new agent to start working immediately
- Known issues should reference specific files/lines where TODOs or problems exist
- If you made architectural decisions, also record them with ` + "`adaf decision create`" + ` and mention them in your --decisions field
- Use ` + "`adaf log search --query \"keyword\"`" + ` to find relevant prior session context when needed

### Decisions
- ` + "`adaf decision list`" + ` — Review past architectural decisions
- ` + "`adaf decision create --title \"...\" --context \"...\" --decision \"...\" --rationale \"...\"`" + ` — Record a new decision

### Documents
- ` + "`adaf doc list`" + ` — List project documents
- ` + "`adaf doc show <id>`" + ` — Read a document

### Agent Orchestration (when delegation is enabled for your loop step)

**Required flow — do NOT use ` + "`--wait`" + `:**
1. Spawn all independent tasks at once (without ` + "`--wait`" + `)
2. Call ` + "`adaf wait-for-spawns`" + ` once
3. Finish your turn immediately — you resume automatically when children complete

` + "`wait-for-spawns`" + ` suspends your session at zero token cost. ` + "`--wait`" + ` keeps your session alive and wastes tokens. Only use ` + "`--wait`" + ` when you need a child's output to spawn the next task in the same turn.

Use ` + "`--read-only`" + ` scouts for any information gathering (repo structure, file contents, git history, test status). Note: read-only scouts run in an isolated worktree snapshot at HEAD — they do NOT see uncommitted or staged changes.

- ` + "`adaf spawn --profile <name> [--role <role>] --task \"...\" [--read-only]`" + ` — Spawn a sub-agent (non-blocking)
- ` + "`adaf spawn --profile <name> [--role <role>] --task-file <path> [--read-only]`" + ` — Spawn with detailed task from file
- ` + "`adaf wait-for-spawns`" + ` — Suspend until all spawns complete (zero-cost wait)
- ` + "`adaf spawn-status [--spawn-id N]`" + ` — Check spawn status
- ` + "`adaf spawn-diff --spawn-id N`" + ` — View diff of spawn's changes
- ` + "`adaf spawn-merge --spawn-id N [--squash]`" + ` — Merge spawn's changes
- ` + "`adaf spawn-reject --spawn-id N`" + ` — Reject spawn's changes
- ` + "`adaf spawn-message --spawn-id N \"guidance\"`" + ` — Send async message to running child
- ` + "`adaf spawn-message --spawn-id N --interrupt \"new instructions\"`" + ` — Redirect a running child

### Supervisor Notes
- ` + "`adaf note add --session N --note \"guidance text\"`" + ` — Send a note to a running session
- ` + "`adaf note list [--session N]`" + ` — List supervisor notes

### Worktree Management
- ` + "`adaf worktree list`" + ` — List active adaf-managed worktrees
- ` + "`adaf worktree cleanup`" + ` — Remove all adaf-managed worktrees (crash recovery)

## Session Protocol

1. **Orient**: Run ` + "`adaf status`" + ` and ` + "`adaf log latest`" + ` to understand current state. If you need more history, use ` + "`adaf log list`" + ` and ` + "`adaf log show <id>`" + ` or ` + "`adaf log search --query \"...\"`" + `
2. **Decide**: Pick the highest-impact work based on the plan, open issues, and the previous session's next steps
3. **Work**: Build, test, integrate. Run tests frequently. Ensure ` + "`go build`" + ` passes before moving on
4. **Record decisions**: If you made architectural choices, run ` + "`adaf decision create --title \"...\" --context \"...\" --decision \"...\" --rationale \"...\"`" + `
5. **Log**: Write a detailed session log with ` + "`adaf log create`" + ` — include ALL fields, especially --build-state, --next, and --issues
6. **Commit**: Commit your code changes
`
}

// PromptTemplates defines common prompt patterns for different agent types.
var PromptTemplates = map[string]string{
	"dot":      ".",
	"orient":   "Read the project status and latest session log, then decide what to work on next. Start working immediately.",
	"fix":      "Check for any failing tests or build errors and fix them.",
	"continue": "Continue working on the current in-progress phase of the plan.",
}
