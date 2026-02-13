package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/store"
)

func TestFitLinesTruncatesWithoutWrapping(t *testing.T) {
	width := 10
	height := 3

	plainLong := strings.Repeat("x", 20)
	styledLong := lipgloss.NewStyle().Foreground(ColorMauve).Render(strings.Repeat("y", 20))

	out := fitLines([]string{plainLong, styledLong, "ok"}, width, height)
	lines := splitRenderableLines(out)
	if len(lines) != height {
		t.Fatalf("line count = %d, want %d", len(lines), height)
	}

	if got, want := ansi.Strip(lines[0]), strings.Repeat("x", width); got != want {
		t.Fatalf("line[0] = %q, want %q", got, want)
	}
	if got, want := ansi.Strip(lines[1]), strings.Repeat("y", width); got != want {
		t.Fatalf("line[1] = %q, want %q", got, want)
	}
	if got, want := ansi.Strip(lines[2]), "ok"+strings.Repeat(" ", width-2); got != want {
		t.Fatalf("line[2] = %q, want %q", got, want)
	}

	for i, line := range lines {
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("line[%d] width = %d, want %d", i, got, width)
		}
	}
}

func TestFitLinesWithOffsetWindowsContent(t *testing.T) {
	width := 6
	height := 3
	lines := []string{"row0", "row1", "row2", "row3", "row4"}

	out := fitLinesWithOffset(lines, width, height, 2)
	got := splitRenderableLines(out)
	if len(got) != height {
		t.Fatalf("line count = %d, want %d", len(got), height)
	}

	if stripped := ansi.Strip(got[0]); stripped != "row2  " {
		t.Fatalf("line[0] = %q, want %q", stripped, "row2  ")
	}
	if stripped := ansi.Strip(got[2]); stripped != "row4  " {
		t.Fatalf("line[2] = %q, want %q", stripped, "row4  ")
	}
}

func TestFitLinesWithCursorKeepsSelectionVisible(t *testing.T) {
	width := 6
	height := 3
	lines := []string{"row0", "row1", "row2", "row3", "row4", "row5"}

	out := fitLinesWithCursor(lines, width, height, 5)
	got := splitRenderableLines(out)
	if len(got) != height {
		t.Fatalf("line count = %d, want %d", len(got), height)
	}

	if stripped := ansi.Strip(got[0]); stripped != "row3  " {
		t.Fatalf("line[0] = %q, want %q", stripped, "row3  ")
	}
	if stripped := ansi.Strip(got[2]); stripped != "row5  " {
		t.Fatalf("line[2] = %q, want %q", stripped, "row5  ")
	}
}

func TestRenderSelectorKeepsFixedPanelHeight(t *testing.T) {
	profiles := []profileEntry{
		{
			Name:  "alpha",
			Agent: "codex",
			Model: strings.Repeat("gpt-5-codex-", 10),
			Caps: []string{
				"plan", "edit", "tools", strings.Repeat("feature-", 12),
			},
		},
		{
			Name:  "beta",
			Agent: "claude",
			Model: strings.Repeat("claude-sonnet-", 8),
			Caps: []string{
				"reasoning", "files", strings.Repeat("capability-", 10),
			},
		},
		{
			Name:  "gamma",
			Agent: "gemini",
			Model: strings.Repeat("gemini-2.5-pro-", 8),
			Caps: []string{
				"stream", "exec", strings.Repeat("tooling-", 14),
			},
		},
		{IsSeparator: true, Name: "───"},
		{IsNew: true, Name: "+ New Profile"},
		{IsNewLoop: true, Name: "+ New Loop"},
	}

	project := &store.ProjectConfig{
		Name:     strings.Repeat("very-long-project-name-", 8),
		RepoPath: "/tmp/repo",
	}

	width := 80
	height := 18
	wantPanelHeight := height - 2

	for selected := range profiles {
		out := renderSelector(
			profiles,
			selected,
			project,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			width,
			height,
			0,
			false,
		)
		if got := len(splitRenderableLines(out)); got != wantPanelHeight {
			t.Fatalf("selected=%d panel height = %d, want %d", selected, got, wantPanelHeight)
		}
	}
}
