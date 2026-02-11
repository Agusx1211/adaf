package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// logsView renders the session logs view.
func (m Model) logsView() string {
	width := m.width
	if width < 20 {
		width = 80
	}
	contentWidth := width - 4

	if len(m.logs) == 0 {
		return EmptyStateStyle.Render("No session logs yet. Logs are created after each agent session.")
	}

	// Summary
	summary := lipgloss.NewStyle().Foreground(ColorSubtext0).Render(
		fmt.Sprintf("  %d session logs recorded", len(m.logs)),
	)

	if m.logShowDetail && m.logSelectedIdx >= 0 && m.logSelectedIdx < len(m.logs) {
		return lipgloss.JoinVertical(lipgloss.Left,
			summary, "",
			m.renderLogDetail(contentWidth),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		summary, "",
		m.renderLogsList(contentWidth),
	)
}

func (m Model) renderLogsList(width int) string {
	var lines []string

	// Table header
	hdr := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	headerLine := fmt.Sprintf("  %-5s  %-12s  %-10s  %-12s  %s",
		hdr.Render("ID"),
		hdr.Render("Date"),
		hdr.Render("Agent"),
		hdr.Render("Build"),
		hdr.Render("Objective"),
	)
	lines = append(lines, headerLine)
	lines = append(lines, DividerStyle.Render(strings.Repeat("─", width-4)))

	maxVisible := m.height - 14
	if maxVisible < 5 {
		maxVisible = 5
	}

	// Show most recent first
	startIdx := 0
	if m.logSelectedIdx >= maxVisible {
		startIdx = m.logSelectedIdx - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.logs) {
		endIdx = len(m.logs)
	}

	for i := startIdx; i < endIdx; i++ {
		// Display in reverse order (most recent first) - index into reversed view
		revIdx := len(m.logs) - 1 - i
		if revIdx < 0 {
			break
		}
		l := m.logs[revIdx]
		isSelected := i == m.logSelectedIdx

		agentStyle := lipgloss.NewStyle().Foreground(ColorTeal).Bold(true)
		dateStr := l.Date.Format("01/02 15:04")

		buildStyle := lipgloss.NewStyle().Foreground(ColorGreen)
		buildState := l.BuildState
		switch buildState {
		case "passing", "pass", "ok":
			buildStyle = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
		case "failing", "fail", "error":
			buildStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
		case "unknown", "":
			buildStyle = lipgloss.NewStyle().Foreground(ColorOverlay0)
			buildState = "unknown"
		default:
			buildStyle = lipgloss.NewStyle().Foreground(ColorYellow)
		}

		obj := l.Objective
		maxObjLen := width - 52
		if maxObjLen < 10 {
			maxObjLen = 10
		}
		if len(obj) > maxObjLen {
			obj = obj[:maxObjLen-3] + "..."
		}

		line := fmt.Sprintf("  %-5d  %-12s  %-10s  %-12s  %s",
			l.ID,
			lipgloss.NewStyle().Foreground(ColorOverlay0).Render(dateStr),
			agentStyle.Render(l.Agent),
			buildStyle.Render(buildState),
			lipgloss.NewStyle().Foreground(ColorText).Render(obj),
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

	if len(m.logs) > maxVisible {
		scrollInfo := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(
			fmt.Sprintf("  showing %d-%d of %d", startIdx+1, endIdx, len(m.logs)),
		)
		lines = append(lines, "", scrollInfo)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderLogDetail(width int) string {
	// Reverse index mapping
	revIdx := len(m.logs) - 1 - m.logSelectedIdx
	if revIdx < 0 || revIdx >= len(m.logs) {
		return ErrorStyle.Render("Invalid log index")
	}
	l := m.logs[revIdx]

	backHint := lipgloss.NewStyle().Foreground(ColorOverlay0).Italic(true).Render("  Press ESC to go back")

	title := DetailTitleStyle.Render(fmt.Sprintf("Session #%d", l.ID))

	dateLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Date:"), l.Date.Format("2006-01-02 15:04:05"))
	agentLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Agent:"),
		lipgloss.NewStyle().Foreground(ColorTeal).Bold(true).Render(l.Agent))

	var modelLine string
	if l.AgentModel != "" {
		modelLine = fmt.Sprintf("%s %s", DetailLabelStyle.Render("Model:"), l.AgentModel)
	}

	var commitLine string
	if l.CommitHash != "" {
		commitLine = fmt.Sprintf("%s %s", DetailLabelStyle.Render("Commit:"),
			lipgloss.NewStyle().Foreground(ColorPeach).Render(l.CommitHash))
	}

	var durationLine string
	if l.DurationSecs > 0 {
		mins := l.DurationSecs / 60
		secs := l.DurationSecs % 60
		durationLine = fmt.Sprintf("%s %dm %ds", DetailLabelStyle.Render("Duration:"), mins, secs)
	}

	buildStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	switch l.BuildState {
	case "passing", "pass", "ok":
		buildStyle = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	case "failing", "fail", "error":
		buildStyle = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
	default:
		buildStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	}
	buildLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Build:"), buildStyle.Render(l.BuildState))

	sections := []string{backHint, "", title, dateLine, agentLine}
	if modelLine != "" {
		sections = append(sections, modelLine)
	}
	if commitLine != "" {
		sections = append(sections, commitLine)
	}
	if durationLine != "" {
		sections = append(sections, durationLine)
	}
	sections = append(sections, buildLine, "")

	// Content sections
	contentSections := []struct {
		label   string
		content string
	}{
		{"Objective", l.Objective},
		{"What Was Built", l.WhatWasBuilt},
		{"Key Decisions", l.KeyDecisions},
		{"Challenges", l.Challenges},
		{"Current State", l.CurrentState},
		{"Known Issues", l.KnownIssues},
		{"Next Steps", l.NextSteps},
	}

	maxContentWidth := width - 6
	if maxContentWidth < 20 {
		maxContentWidth = 20
	}

	for _, cs := range contentSections {
		if cs.content == "" {
			continue
		}
		header := DetailLabelStyle.Render(cs.label + ":")
		body := DetailContentStyle.Width(maxContentWidth).Render(cs.content)
		sections = append(sections, header, body, "")
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return CardStyle.Width(width).Render(content)
}
