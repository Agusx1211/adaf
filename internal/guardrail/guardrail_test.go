package guardrail

import (
	"encoding/json"
	"testing"

	"github.com/agusx1211/adaf/internal/stream"
)

func TestNewMonitor(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		enabled bool
		wantNil bool
	}{
		{"manager enabled", "manager", true, false},
		{"supervisor enabled", "supervisor", true, false},
		{"senior enabled", "senior", true, true}, // can write code
		{"junior enabled", "junior", true, true}, // can write code
		{"manager disabled", "manager", false, true},
		{"empty role enabled", "", true, true}, // defaults to junior
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor(tt.role, tt.enabled)
			if (m == nil) != tt.wantNil {
				t.Errorf("NewMonitor(%q, %v) nil=%v, want nil=%v", tt.role, tt.enabled, m == nil, tt.wantNil)
			}
		})
	}
}

func TestCheckEvent_WriteTools(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{"Write tool", "Write", "Write"},
		{"Edit tool", "Edit", "Edit"},
		{"MultiEdit tool", "MultiEdit", "MultiEdit"},
		{"NotebookEdit tool", "NotebookEdit", "NotebookEdit"},
		{"write_file tool", "write_file", "write_file"},
		{"edit_file tool", "edit_file", "edit_file"},
		{"replace tool", "replace", "replace"},
		{"search_replace tool", "search_replace", "search_replace"},
		{"Read tool (no violation)", "Read", ""},
		{"Grep tool (no violation)", "Grep", ""},
		{"Glob tool (no violation)", "Glob", ""},
		{"Task tool (no violation)", "Task", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor("manager", true)
			ev := stream.ClaudeEvent{
				Type: "assistant",
				AssistantMessage: &stream.AssistantMessage{
					Content: []stream.ContentBlock{
						{Type: "tool_use", Name: tt.toolName},
					},
				},
			}
			got := m.CheckEvent(ev)
			if got != tt.want {
				t.Errorf("CheckEvent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckEvent_BashCommands(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"redirect", "echo hello > file.txt", "Bash(>)"},
		{"append redirect", "echo hello >> file.txt", "Bash(>)"},
		{"sed in-place", "sed -i 's/a/b/' file.txt", "Bash(sed -i)"},
		{"mv command", "mv old.txt new.txt", "Bash(mv )"},
		{"rm command", "rm file.txt", "Bash(rm )"},
		{"mkdir command", "mkdir -p foo/bar", "Bash(mkdir )"},
		{"touch command", "touch new.txt", "Bash(touch )"},
		{"cp command", "cp a.txt b.txt", "Bash(cp )"},
		{"chmod command", "chmod 755 script.sh", "Bash(chmod )"},
		{"tee command", "echo hello | tee file.txt", "Bash(tee )"},
		{"adaf command (allowed)", "adaf spawn-status --spawn-id 1", ""},
		{"adaf loop stop (allowed)", "adaf loop stop", ""},
		{"git status (allowed)", "git status", ""},
		{"git diff (allowed)", "git diff HEAD", ""},
		{"ls (allowed)", "ls -la", ""},
		{"cat (allowed)", "cat file.txt", ""},
		{"grep (allowed)", "grep -r pattern .", ""},
		{"empty command", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitor("manager", true)
			input, _ := json.Marshal(map[string]string{"command": tt.command})
			ev := stream.ClaudeEvent{
				Type: "assistant",
				AssistantMessage: &stream.AssistantMessage{
					Content: []stream.ContentBlock{
						{Type: "tool_use", Name: "Bash", Input: input},
					},
				},
			}
			got := m.CheckEvent(ev)
			if got != tt.want {
				t.Errorf("CheckEvent(Bash %q) = %q, want %q", tt.command, got, tt.want)
			}
		})
	}
}

func TestCheckEvent_StreamingBlocks(t *testing.T) {
	m := NewMonitor("supervisor", true)

	// content_block_start with Write tool
	ev := stream.ClaudeEvent{
		Type: "content_block_start",
		ContentBlock: &stream.ContentBlock{
			Type: "tool_use",
			Name: "Write",
		},
	}
	got := m.CheckEvent(ev)
	if got != "Write" {
		t.Errorf("streaming content_block_start Write: got %q, want %q", got, "Write")
	}

	// content_block_start with Read tool (no violation)
	ev2 := stream.ClaudeEvent{
		Type: "content_block_start",
		ContentBlock: &stream.ContentBlock{
			Type: "tool_use",
			Name: "Read",
		},
	}
	got2 := m.CheckEvent(ev2)
	if got2 != "" {
		t.Errorf("streaming content_block_start Read: got %q, want empty", got2)
	}
}

func TestCheckEvent_TextBlocks(t *testing.T) {
	m := NewMonitor("manager", true)

	// Text block should not trigger violation.
	ev := stream.ClaudeEvent{
		Type: "assistant",
		AssistantMessage: &stream.AssistantMessage{
			Content: []stream.ContentBlock{
				{Type: "text", Text: "I will edit the file now..."},
			},
		},
	}
	got := m.CheckEvent(ev)
	if got != "" {
		t.Errorf("text block should not trigger violation, got %q", got)
	}

	// Thinking block.
	ev2 := stream.ClaudeEvent{
		Type: "content_block_start",
		ContentBlock: &stream.ContentBlock{
			Type: "thinking",
		},
	}
	got2 := m.CheckEvent(ev2)
	if got2 != "" {
		t.Errorf("thinking block should not trigger violation, got %q", got2)
	}
}

func TestViolationCounter(t *testing.T) {
	m := NewMonitor("manager", true)
	if m.Violations() != 0 {
		t.Fatalf("initial violations = %d, want 0", m.Violations())
	}

	ev := stream.ClaudeEvent{
		Type: "content_block_start",
		ContentBlock: &stream.ContentBlock{
			Type: "tool_use",
			Name: "Write",
		},
	}

	m.CheckEvent(ev)
	if m.Violations() != 1 {
		t.Errorf("after first violation: got %d, want 1", m.Violations())
	}

	ev.ContentBlock.Name = "Edit"
	m.CheckEvent(ev)
	if m.Violations() != 2 {
		t.Errorf("after second violation: got %d, want 2", m.Violations())
	}

	// Non-violation should not increment.
	ev.ContentBlock.Name = "Read"
	m.CheckEvent(ev)
	if m.Violations() != 2 {
		t.Errorf("after non-violation: got %d, want 2", m.Violations())
	}
}

func TestNilMonitor(t *testing.T) {
	var m *Monitor
	if m.Violations() != 0 {
		t.Errorf("nil monitor Violations() = %d, want 0", m.Violations())
	}
	got := m.CheckEvent(stream.ClaudeEvent{Type: "assistant"})
	if got != "" {
		t.Errorf("nil monitor CheckEvent() = %q, want empty", got)
	}
}

func TestWarningMessage(t *testing.T) {
	msg := WarningMessage("manager", "Write", 1)
	if msg == "" {
		t.Fatal("WarningMessage returned empty string")
	}
	if !contains(msg, "GUARDRAIL VIOLATION #1") {
		t.Errorf("message missing violation number: %s", msg)
	}
	if !contains(msg, "manager") {
		t.Errorf("message missing role: %s", msg)
	}
	if !contains(msg, "Write") {
		t.Errorf("message missing tool name: %s", msg)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsHelper(s, sub))
}

func containsHelper(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
