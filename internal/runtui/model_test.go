package runtui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

func TestWrapRenderableLinesWordWrap(t *testing.T) {
	lines := wrapRenderableLines([]string{"one two three four"}, 7)
	if len(lines) != 3 {
		t.Fatalf("wrapped line count = %d, want 3", len(lines))
	}

	got := make([]string, 0, len(lines))
	for _, line := range lines {
		got = append(got, ansi.Strip(line))
	}

	want := []string{"one two", "three", "four"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDetailLinesWrapsWithoutTruncating(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any), nil)
	m.SetSize(96, 20)
	width := m.rcWidth()
	long := strings.Repeat("x", width+17)
	m.addLine(long)

	lines := m.detailLines(width)
	if len(lines) < 2 {
		t.Fatalf("wrapped line count = %d, want >= 2", len(lines))
	}

	var combined strings.Builder
	for i, line := range lines {
		plain := ansi.Strip(line)
		if lipgloss.Width(plain) > width {
			t.Fatalf("line[%d] width = %d, want <= %d", i, lipgloss.Width(plain), width)
		}
		combined.WriteString(plain)
	}

	if combined.String() != long {
		t.Errorf("wrapped text mismatch: got len=%d want len=%d", len(combined.String()), len(long))
	}
}

func TestDetachMessageQuitsInSessionMode(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.SetSessionMode(7)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatalf("ctrl+d command = nil, want detach command")
	}
	detachMsg := cmd()
	detach, ok := detachMsg.(DetachMsg)
	if !ok {
		t.Fatalf("ctrl+d msg type = %T, want DetachMsg", detachMsg)
	}
	if detach.SessionID != 7 {
		t.Fatalf("detach session id = %d, want 7", detach.SessionID)
	}

	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	_, quitCmd := m2.Update(detach)
	if quitCmd == nil {
		t.Fatalf("quit command = nil, want tea.Quit")
	}
	quitMsg := quitCmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("quit command result type = %T, want tea.QuitMsg", quitMsg)
	}
}

func TestTurnTerminologyInRunPanel(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	updated, _ := m.Update(AgentStartedMsg{SessionID: 42})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	if len(m2.lines) == 0 {
		t.Fatal("expected start line to be recorded")
	}
	last := ansi.Strip(m2.lines[len(m2.lines)-1].text)
	if !strings.Contains(last, "Turn #42 started") {
		t.Fatalf("start line = %q, want to contain %q", last, "Turn #42 started")
	}

	updated, _ = m2.Update(AgentFinishedMsg{
		SessionID: 42,
		Result: &agent.Result{
			ExitCode: 0,
			Duration: time.Second,
		},
	})
	m3, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	foundFinish := false
	for _, line := range m3.lines {
		if strings.Contains(ansi.Strip(line.text), "Turn #42 finished") {
			foundFinish = true
			break
		}
	}
	if !foundFinish {
		t.Fatal("expected finish line to use Turn terminology")
	}

	entries := m3.commandEntries()
	foundTurnEntry := false
	for _, entry := range entries {
		if strings.Contains(entry.title, "turn #42") {
			foundTurnEntry = true
			break
		}
	}
	if !foundTurnEntry {
		t.Fatal("expected command center entry title to use turn #")
	}

	prefix := m3.scopePrefix("session:42")
	if !strings.Contains(prefix, "turn #42") {
		t.Fatalf("scope prefix = %q, want to contain %q", prefix, "turn #42")
	}
}

func TestIssueAndDocDetailModes(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.issues = []store.Issue{
		{
			ID:          7,
			Title:       "Fix flaky test",
			Description: "Repro:\n- run go test ./...",
			Status:      "open",
			Priority:    "high",
			Created:     time.Now().Add(-2 * time.Hour),
			Updated:     time.Now().Add(-time.Hour),
		},
	}
	m.docs = []store.Doc{
		{
			ID:      "arch",
			Title:   "Architecture",
			Content: "System overview\nComponent map",
			Created: time.Now().Add(-3 * time.Hour),
			Updated: time.Now().Add(-30 * time.Minute),
		},
	}

	m.leftSection = leftSectionIssues
	issueLines := strings.Join(stripStyledLines(m.detailLines(80)), "\n")
	if !strings.Contains(issueLines, "Issue #7") {
		t.Fatalf("issue detail did not render selected issue, got: %q", issueLines)
	}
	if !strings.Contains(issueLines, "Fix flaky test") {
		t.Fatalf("issue detail did not include title, got: %q", issueLines)
	}

	m.leftSection = leftSectionDocs
	docLines := strings.Join(stripStyledLines(m.detailLines(80)), "\n")
	if !strings.Contains(docLines, "Doc arch") {
		t.Fatalf("doc detail did not render selected doc, got: %q", docLines)
	}
	if !strings.Contains(docLines, "System overview") {
		t.Fatalf("doc detail did not include content, got: %q", docLines)
	}
}

func TestIssueSelectionWithKeyboard(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.focus = focusCommand
	m.leftSection = leftSectionIssues
	m.issues = []store.Issue{
		{ID: 1, Title: "one", Status: "open", Priority: "low"},
		{ID: 2, Title: "two", Status: "open", Priority: "low"},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	if got.selectedIssue != 1 {
		t.Fatalf("selectedIssue = %d, want 1", got.selectedIssue)
	}
	if got.selectedScope() != "" {
		t.Fatalf("selectedScope = %q, want empty in issue mode", got.selectedScope())
	}
}

func TestStreamingDeltaAfterModelCopyDoesNotPanic(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	// Bubble Tea app models are value types; this simulates parent-model copying.
	copied := m

	start := AgentEventMsg{
		Event: stream.ClaudeEvent{
			Type:         "content_block_start",
			ContentBlock: &stream.ContentBlock{Type: "text"},
		},
	}
	delta := AgentEventMsg{
		Event: stream.ClaudeEvent{
			Type:  "content_block_delta",
			Delta: &stream.Delta{Text: "hello"},
		},
	}

	updated, _ := copied.Update(start)
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	updated, _ = m2.Update(delta)
	m3, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic after model copy: %v", r)
		}
	}()

	updated, _ = m3.Update(delta)
	m4, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	if m4.streamBuf == nil || m4.streamBuf.Len() == 0 {
		t.Fatal("expected streaming buffer to contain accumulated delta text")
	}
}

func stripStyledLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, ansi.Strip(line))
	}
	return out
}
