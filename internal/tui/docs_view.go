package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// docsView renders the documentation view.
func (m Model) docsView() string {
	width := m.width
	if width < 20 {
		width = 80
	}
	contentWidth := width - 4

	if len(m.docs) == 0 {
		return EmptyStateStyle.Render("No documentation found. Docs are created as the project evolves.")
	}

	summary := lipgloss.NewStyle().Foreground(ColorSubtext0).Render(
		fmt.Sprintf("  %d documents", len(m.docs)),
	)

	if m.docShowDetail && m.docSelectedIdx >= 0 && m.docSelectedIdx < len(m.docs) {
		return lipgloss.JoinVertical(lipgloss.Left,
			summary, "",
			m.renderDocDetail(contentWidth),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		summary, "",
		m.renderDocsList(contentWidth),
	)
}

func (m Model) renderDocsList(width int) string {
	var lines []string

	hdr := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	headerLine := fmt.Sprintf("  %-8s  %-40s  %-14s  %s",
		hdr.Render("ID"),
		hdr.Render("Title"),
		hdr.Render("Updated"),
		hdr.Render("Size"),
	)
	lines = append(lines, headerLine)
	lines = append(lines, DividerStyle.Render(strings.Repeat("─", width-4)))

	maxVisible := m.height - 14
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if m.docSelectedIdx >= maxVisible {
		startIdx = m.docSelectedIdx - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.docs) {
		endIdx = len(m.docs)
	}

	for i := startIdx; i < endIdx; i++ {
		doc := m.docs[i]
		isSelected := i == m.docSelectedIdx

		title := doc.Title
		maxTitleLen := 40
		if len(title) > maxTitleLen {
			title = title[:maxTitleLen-3] + "..."
		}

		dateStr := doc.Updated.Format("01/02 15:04")

		// Show content size in a human-readable way
		size := len(doc.Content)
		var sizeStr string
		if size < 1024 {
			sizeStr = fmt.Sprintf("%d B", size)
		} else {
			sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
		}

		line := fmt.Sprintf("  %-8s  %-40s  %-14s  %s",
			lipgloss.NewStyle().Foreground(ColorBlue).Render(doc.ID),
			lipgloss.NewStyle().Foreground(ColorText).Render(title),
			lipgloss.NewStyle().Foreground(ColorOverlay0).Render(dateStr),
			lipgloss.NewStyle().Foreground(ColorSubtext0).Render(sizeStr),
		)

		if isSelected {
			line = lipgloss.NewStyle().
				Background(ColorSurface1).
				Foreground(ColorText).
				Bold(true).
				Width(width - 2).
				Render(line)
		}

		lines = append(lines, line)
	}

	if len(m.docs) > maxVisible {
		scrollInfo := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(
			fmt.Sprintf("  showing %d-%d of %d", startIdx+1, endIdx, len(m.docs)),
		)
		lines = append(lines, "", scrollInfo)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderDocDetail(width int) string {
	doc := m.docs[m.docSelectedIdx]

	backHint := lipgloss.NewStyle().Foreground(ColorOverlay0).Italic(true).Render("  Press ESC to go back")

	title := DetailTitleStyle.Render(doc.Title)

	idLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("ID:"),
		lipgloss.NewStyle().Foreground(ColorBlue).Render(doc.ID))
	createdLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Created:"),
		doc.Created.Format("2006-01-02 15:04:05"))
	updatedLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Updated:"),
		doc.Updated.Format("2006-01-02 15:04:05"))

	contentHeader := DetailLabelStyle.Render("Content:")

	// Render content with word wrapping
	maxContentWidth := width - 6
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}

	// Limit displayed content to avoid overwhelming the terminal
	content := doc.Content
	maxLines := m.height - 20
	if maxLines < 10 {
		maxLines = 10
	}
	contentLines := strings.Split(content, "\n")
	truncated := false
	if len(contentLines) > maxLines {
		contentLines = contentLines[:maxLines]
		truncated = true
	}
	content = strings.Join(contentLines, "\n")

	renderedContent := DetailContentStyle.Width(maxContentWidth).Render(content)

	sections := []string{backHint, "", title, idLine, createdLine, updatedLine, "", contentHeader, renderedContent}

	if truncated {
		sections = append(sections, "",
			lipgloss.NewStyle().Foreground(ColorOverlay0).Italic(true).Render(
				fmt.Sprintf("  ... content truncated (%d more lines)", len(strings.Split(doc.Content, "\n"))-maxLines),
			),
		)
	}

	result := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return CardStyle.Width(width).Render(result)
}
