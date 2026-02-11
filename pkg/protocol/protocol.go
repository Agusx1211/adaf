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
- ` + "`adaf log show <id>`" + ` — Read a specific session log

### Issues
- ` + "`adaf issue list`" + ` — List all open issues
- ` + "`adaf issue list --status all`" + ` — List all issues including resolved
- ` + "`adaf issue create --title \"...\" --description \"...\" --priority high`" + ` — Report a new issue
- ` + "`adaf issue update <id> --status resolved`" + ` — Mark an issue as resolved

### Session Logging (do this at the end of every session)
- ` + "`adaf log create --agent <your-name> --objective \"...\" --built \"...\" --state \"...\" --next \"...\"`" + ` — Write your session log

### Decisions
- ` + "`adaf decision list`" + ` — Review past architectural decisions
- ` + "`adaf decision create --title \"...\" --context \"...\" --decision \"...\" --rationale \"...\"`" + ` — Record a new decision

### Documents
- ` + "`adaf doc list`" + ` — List project documents
- ` + "`adaf doc show <id>`" + ` — Read a document

### Agent Orchestration (for manager/senior roles)
- ` + "`adaf spawn --profile <name> --task \"...\" [--read-only] [--wait]`" + ` — Spawn a sub-agent
- ` + "`adaf spawn-status [--spawn-id N]`" + ` — Check spawn status
- ` + "`adaf spawn-wait [--spawn-id N]`" + ` — Wait for spawn(s) to complete
- ` + "`adaf spawn-diff --spawn-id N`" + ` — View diff of spawn's changes
- ` + "`adaf spawn-merge --spawn-id N [--squash]`" + ` — Merge spawn's changes
- ` + "`adaf spawn-reject --spawn-id N`" + ` — Reject spawn's changes

### Supervisor Notes
- ` + "`adaf note add --session N --note \"guidance text\"`" + ` — Send a note to a running session
- ` + "`adaf note list [--session N]`" + ` — List supervisor notes

### Worktree Management
- ` + "`adaf worktree list`" + ` — List active adaf-managed worktrees
- ` + "`adaf worktree cleanup`" + ` — Remove all adaf-managed worktrees (crash recovery)

## Session Protocol

1. **Orient**: Run ` + "`adaf status`" + ` and ` + "`adaf log latest`" + ` to understand current state
2. **Decide**: Pick the highest-impact work based on the plan and open issues
3. **Work**: Build, test, integrate
4. **Log**: Write your session log with ` + "`adaf log create`" + `
5. **Commit**: Commit your code changes
`
}

// PromptTemplates defines common prompt patterns for different agent types.
var PromptTemplates = map[string]string{
	"dot": ".",
	"orient": "Read the project status and latest session log, then decide what to work on next. Start working immediately.",
	"fix": "Check for any failing tests or build errors and fix them.",
	"continue": "Continue working on the current in-progress phase of the plan.",
}
