package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// issuesView renders the issues list view.
func (m Model) issuesView() string {
	width := m.width
	if width < 20 {
		width = 80
	}
	contentWidth := width - 4

	if len(m.issues) == 0 {
		return EmptyStateStyle.Render("No issues found. Issues will appear here as agents report them.")
	}

	// Summary header
	open, inProg, resolved, wontfix := 0, 0, 0, 0
	for _, issue := range m.issues {
		switch issue.Status {
		case "open":
			open++
		case "in_progress":
			inProg++
		case "resolved":
			resolved++
		case "wontfix":
			wontfix++
		}
	}
	summary := fmt.Sprintf("  %s  %s  %s  %s  |  %d total",
		IssueOpen.Render(fmt.Sprintf("%d open", open)),
		IssueInProgress.Render(fmt.Sprintf("%d active", inProg)),
		IssueResolved.Render(fmt.Sprintf("%d resolved", resolved)),
		IssueWontfix.Render(fmt.Sprintf("%d wontfix", wontfix)),
		len(m.issues),
	)

	if m.issueShowDetail && m.issueSelectedIdx >= 0 && m.issueSelectedIdx < len(m.issues) {
		return lipgloss.JoinVertical(lipgloss.Left,
			summary, "",
			m.renderIssueDetail(contentWidth),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		summary, "",
		m.renderIssuesList(contentWidth),
	)
}

func (m Model) renderIssuesList(width int) string {
	var lines []string

	// Table header
	hdr := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	headerLine := fmt.Sprintf("  %-6s  %-40s  %-12s  %-10s  %s",
		hdr.Render("ID"),
		hdr.Render("Title"),
		hdr.Render("Status"),
		hdr.Render("Priority"),
		hdr.Render("Date"),
	)
	lines = append(lines, headerLine)
	lines = append(lines, DividerStyle.Render(strings.Repeat("─", width-4)))

	maxVisible := m.height - 14
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if m.issueSelectedIdx >= maxVisible {
		startIdx = m.issueSelectedIdx - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.issues) {
		endIdx = len(m.issues)
	}

	for i := startIdx; i < endIdx; i++ {
		issue := m.issues[i]
		isSelected := i == m.issueSelectedIdx

		title := issue.Title
		maxTitleLen := 40
		if len(title) > maxTitleLen {
			title = title[:maxTitleLen-3] + "..."
		}

		statusStr := StyledIssueStatus(issue.Status)
		priorityStr := StyledPriority(issue.Priority)
		dateStr := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(issue.Created.Format("01/02/06"))

		// Pad status and priority to fixed visual widths using plain formatting
		line := fmt.Sprintf("  %-6d  %-40s  %-12s  %-10s  %s",
			issue.ID,
			title,
			statusStr,
			priorityStr,
			dateStr,
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

	if len(m.issues) > maxVisible {
		scrollInfo := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(
			fmt.Sprintf("  showing %d-%d of %d", startIdx+1, endIdx, len(m.issues)),
		)
		lines = append(lines, "", scrollInfo)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderIssueDetail(width int) string {
	issue := m.issues[m.issueSelectedIdx]

	backHint := lipgloss.NewStyle().Foreground(ColorOverlay0).Italic(true).Render("  Press ESC to go back")

	title := DetailTitleStyle.Render(fmt.Sprintf("#%d: %s", issue.ID, issue.Title))

	statusLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Status:"), StyledIssueStatus(issue.Status))
	priorityLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Priority:"), StyledPriority(issue.Priority))
	dateLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Created:"), issue.Created.Format("2006-01-02 15:04"))
	updatedLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Updated:"), issue.Updated.Format("2006-01-02 15:04"))

	var labelsStr string
	if len(issue.Labels) > 0 {
		var labelBadges []string
		for _, label := range issue.Labels {
			labelBadges = append(labelBadges, BadgeStyle.Render(label))
		}
		labelsStr = strings.Join(labelBadges, " ")
	} else {
		labelsStr = lipgloss.NewStyle().Foreground(ColorOverlay0).Render("none")
	}
	labelsLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Labels:"), labelsStr)

	var sessionLine string
	if issue.SessionID > 0 {
		sessionLine = fmt.Sprintf("%s %d", DetailLabelStyle.Render("Session:"), issue.SessionID)
	} else {
		sessionLine = fmt.Sprintf("%s %s", DetailLabelStyle.Render("Session:"), lipgloss.NewStyle().Foreground(ColorOverlay0).Render("none"))
	}

	descHeader := DetailLabelStyle.Render("Description:")
	desc := DetailContentStyle.Width(width - 6).Render(issue.Description)

	content := lipgloss.JoinVertical(lipgloss.Left,
		backHint, "",
		title,
		statusLine,
		priorityLine,
		dateLine,
		updatedLine,
		labelsLine,
		sessionLine, "",
		descHeader,
		desc,
	)

	return CardStyle.Width(width).Render(content)
}
