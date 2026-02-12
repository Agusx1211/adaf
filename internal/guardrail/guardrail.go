// Package guardrail monitors stream events for role-policy violations.
//
// When enabled on a non-code-writing role (manager/supervisor), the monitor
// inspects assistant stream events for write tool usage. On detection it
// returns a warning message that the caller uses to interrupt the current
// turn and inject corrective context into the next turn.
package guardrail

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/stream"
)

// writeToolNames are tool names that modify files on disk.
var writeToolNames = map[string]bool{
	"Write":          true,
	"Edit":           true,
	"MultiEdit":      true,
	"NotebookEdit":   true,
	"write_file":     true,
	"edit_file":      true,
	"replace":        true,
	"search_replace": true,
}

// bashWritePatterns are substrings in Bash commands that indicate a write operation.
var bashWritePatterns = []string{
	">",      // redirect (covers > and >>)
	"sed -i", // in-place sed
	"mv ",
	"rm ",
	"rm\t",
	"mkdir ",
	"touch ",
	"cp ",
	"chmod ",
	"chown ",
	"tee ",
	"install ",
	"ln ",
	"patch ",
}

// Monitor inspects stream events for guardrail violations.
type Monitor struct {
	role       string
	violations int
}

// NewMonitor creates a guardrail monitor for the given role.
// Returns nil if guardrails are not enabled or the role is allowed to write code.
func NewMonitor(role string, enabled bool) *Monitor {
	if !enabled {
		return nil
	}
	if config.CanWriteCode(role) {
		return nil
	}
	return &Monitor{role: role}
}

// Violations returns the number of violations detected so far.
func (m *Monitor) Violations() int {
	if m == nil {
		return 0
	}
	return m.violations
}

// CheckEvent inspects a Claude stream event for write tool usage.
// Returns the violating tool name, or "" if no violation.
func (m *Monitor) CheckEvent(ev stream.ClaudeEvent) string {
	if m == nil {
		return ""
	}

	// Check full assistant messages (non-streaming mode).
	if ev.Type == "assistant" && ev.AssistantMessage != nil {
		for _, block := range ev.AssistantMessage.Content {
			if name := m.checkBlock(block); name != "" {
				m.violations++
				return name
			}
		}
	}

	// Check streaming content_block_start events.
	if ev.Type == "content_block_start" && ev.ContentBlock != nil {
		if name := m.checkBlock(*ev.ContentBlock); name != "" {
			m.violations++
			return name
		}
	}

	return ""
}

// checkBlock inspects a single content block for write tool usage.
func (m *Monitor) checkBlock(block stream.ContentBlock) string {
	if block.Type != "tool_use" {
		return ""
	}

	name := block.Name
	if writeToolNames[name] {
		return name
	}

	// Check Bash commands for write patterns.
	if name == "Bash" || name == "bash" || name == "execute_command" {
		return m.checkBashInput(block.Input)
	}

	return ""
}

// checkBashInput parses the JSON input of a Bash tool call and checks
// the command for write patterns.
func (m *Monitor) checkBashInput(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}

	var data struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &data); err != nil {
		return ""
	}

	cmd := strings.TrimSpace(data.Command)
	if cmd == "" {
		return ""
	}

	// Allow adaf CLI commands — managers need to run these.
	if strings.HasPrefix(cmd, "adaf ") {
		return ""
	}

	for _, pat := range bashWritePatterns {
		if strings.Contains(cmd, pat) {
			return "Bash(" + pat + ")"
		}
	}

	return ""
}

// WarningMessage builds the interrupt warning text injected into the next turn.
func WarningMessage(role, toolName string, violationCount int) string {
	return fmt.Sprintf(
		"**GUARDRAIL VIOLATION #%d**: Your role is **%s** — you are NOT allowed to write or modify files. "+
			"You attempted to use `%s`, which is a write operation. "+
			"This turn was interrupted. Do NOT use write/edit tools. "+
			"Instead, delegate coding tasks to sub-agents or describe the changes needed.",
		violationCount, role, toolName,
	)
}
