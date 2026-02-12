package runtui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/agent"
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
