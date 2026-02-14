package runtui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/theme"
)

func clampSelectedIndex(selected, count int) int {
	if count <= 0 {
		return -1
	}
	if selected < 0 {
		return 0
	}
	if selected >= count {
		return count - 1
	}
	return selected
}

func selectedItem[T any](items []T, selected int) (T, int, bool) {
	var zero T
	if len(items) == 0 {
		return zero, -1, false
	}
	idx := clampSelectedIndex(selected, len(items))
	return items[idx], idx, true
}

func detailEmptyLines(title, emptyMessage string) []string {
	return []string{
		sectionTitleStyle.Render(title),
		"",
		dimStyle.Render(emptyMessage),
	}
}

func (m Model) appendSelectableList(
	lines *[]string,
	sectionTitle, emptyMessage string,
	count, selected int,
	renderItem func(i int, prefix string, titleStyle lipgloss.Style) (titleLine, metaLine string),
) int {
	*lines = append(*lines, sectionTitleStyle.Render(sectionTitle))
	if count == 0 {
		*lines = append(*lines, dimStyle.Render(emptyMessage))
		return -1
	}

	selected = clampSelectedIndex(selected, count)
	selectedTitleStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
	cursorLine := -1
	for i := 0; i < count; i++ {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = selectedTitleStyle
		}
		titleLine, metaLine := renderItem(i, prefix, titleStyle)
		*lines = append(*lines, titleLine)
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		if metaLine != "" {
			*lines = append(*lines, metaLine)
		}
	}
	return cursorLine
}
