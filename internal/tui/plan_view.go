package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// planView renders the plan phases view.
func (m Model) planView() string {
	width := m.width
	if width < 20 {
		width = 80
	}
	contentWidth := width - 4

	if m.plan == nil || len(m.plan.Phases) == 0 {
		return EmptyStateStyle.Render("No plan defined. Use `adaf plan` to create one.")
	}

	// Title and description
	titleLine := DetailTitleStyle.Render(m.plan.Title)
	descLine := lipgloss.NewStyle().Foreground(ColorSubtext0).Render(m.plan.Description)

	// Progress summary
	total := len(m.plan.Phases)
	complete := 0
	for _, p := range m.plan.Phases {
		if p.Status == "complete" {
			complete++
		}
	}
	percentage := 0
	if total > 0 {
		percentage = complete * 100 / total
	}
	barWidth := 30
	if barWidth > contentWidth-20 {
		barWidth = contentWidth - 20
	}
	if barWidth < 5 {
		barWidth = 5
	}
	filledWidth := barWidth * percentage / 100
	emptyWidth := barWidth - filledWidth
	bar := ProgressBarFilled.Render(strings.Repeat("█", filledWidth)) +
		ProgressBarEmpty.Render(strings.Repeat("░", emptyWidth))
	progressLine := fmt.Sprintf("  %s %s (%d/%d phases)",
		bar,
		lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render(fmt.Sprintf("%d%%", percentage)),
		complete, total,
	)

	header := lipgloss.JoinVertical(lipgloss.Left, titleLine, descLine, "", progressLine, "")

	if m.planShowDetail && m.planSelectedIdx >= 0 && m.planSelectedIdx < len(m.plan.Phases) {
		// Show detail view for selected phase
		return lipgloss.JoinVertical(lipgloss.Left, header, m.renderPhaseDetail(contentWidth))
	}

	// Render phase list
	return lipgloss.JoinVertical(lipgloss.Left, header, m.renderPhaseList(contentWidth))
}

func (m Model) renderPhaseList(width int) string {
	var lines []string

	// Column header
	hdrStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	headerLine := fmt.Sprintf("  %s  %-4s  %-40s  %-12s  %s",
		hdrStyle.Render(" "),
		hdrStyle.Render("#"),
		hdrStyle.Render("Phase"),
		hdrStyle.Render("Status"),
		hdrStyle.Render("Priority"),
	)
	lines = append(lines, headerLine)
	lines = append(lines, DividerStyle.Render(strings.Repeat("─", width-4)))

	maxVisible := m.height - 16 // leave room for header, status bar, etc.
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if m.planSelectedIdx >= maxVisible {
		startIdx = m.planSelectedIdx - maxVisible + 1
	}

	endIdx := startIdx + maxVisible
	if endIdx > len(m.plan.Phases) {
		endIdx = len(m.plan.Phases)
	}

	for i := startIdx; i < endIdx; i++ {
		phase := m.plan.Phases[i]
		isSelected := i == m.planSelectedIdx

		statusIcon := PhaseStatusIndicator(phase.Status)

		statusText := phase.Status
		var statusStyle lipgloss.Style
		switch phase.Status {
		case "complete":
			statusStyle = lipgloss.NewStyle().Foreground(ColorGreen)
			statusText = "Complete"
		case "in_progress":
			statusStyle = lipgloss.NewStyle().Foreground(ColorYellow)
			statusText = "Active"
		case "blocked":
			statusStyle = lipgloss.NewStyle().Foreground(ColorRed)
			statusText = "Blocked"
		default:
			statusStyle = lipgloss.NewStyle().Foreground(ColorOverlay0)
			statusText = "Pending"
		}

		priorityStr := fmt.Sprintf("P%d", phase.Priority)

		title := phase.Title
		maxTitleLen := 40
		if len(title) > maxTitleLen {
			title = title[:maxTitleLen-3] + "..."
		}

		line := fmt.Sprintf("  %s  %-4s  %-40s  %-12s  %s",
			statusIcon,
			lipgloss.NewStyle().Foreground(ColorOverlay0).Render(phase.ID),
			title,
			statusStyle.Render(statusText),
			lipgloss.NewStyle().Foreground(ColorPeach).Render(priorityStr),
		)

		if isSelected {
			// Highlight the selected row
			line = lipgloss.NewStyle().
				Background(ColorSurface1).
				Foreground(ColorText).
				Bold(true).
				Width(width - 2).
				Render(line)
		}

		lines = append(lines, line)
	}

	// Scroll indicator
	if len(m.plan.Phases) > maxVisible {
		scrollInfo := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(
			fmt.Sprintf("  showing %d-%d of %d (scroll with j/k)", startIdx+1, endIdx, len(m.plan.Phases)),
		)
		lines = append(lines, "", scrollInfo)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderPhaseDetail(width int) string {
	phase := m.plan.Phases[m.planSelectedIdx]

	backHint := lipgloss.NewStyle().Foreground(ColorOverlay0).Italic(true).Render("  Press ESC to go back")

	title := DetailTitleStyle.Render(fmt.Sprintf("%s %s", PhaseStatusIndicator(phase.Status), phase.Title))

	idLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("ID:"), phase.ID)

	statusText := phase.Status
	switch phase.Status {
	case "complete":
		statusText = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("Complete")
	case "in_progress":
		statusText = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Render("In Progress")
	case "blocked":
		statusText = lipgloss.NewStyle().Foreground(ColorRed).Bold(true).Render("Blocked")
	default:
		statusText = lipgloss.NewStyle().Foreground(ColorOverlay0).Render("Not Started")
	}
	statusLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Status:"), statusText)

	priorityLine := fmt.Sprintf("%s P%d", DetailLabelStyle.Render("Priority:"), phase.Priority)

	var depsStr string
	if len(phase.DependsOn) > 0 {
		depsStr = strings.Join(phase.DependsOn, ", ")
	} else {
		depsStr = lipgloss.NewStyle().Foreground(ColorOverlay0).Render("none")
	}
	depsLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Depends On:"), depsStr)

	descHeader := DetailLabelStyle.Render("Description:")
	desc := DetailContentStyle.Width(width - 6).Render(phase.Description)

	content := lipgloss.JoinVertical(lipgloss.Left,
		backHint, "",
		title,
		idLine,
		statusLine,
		priorityLine,
		depsLine, "",
		descHeader,
		desc,
	)

	return CardStyle.Width(width).Render(content)
}
