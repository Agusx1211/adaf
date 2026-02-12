package runtui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
