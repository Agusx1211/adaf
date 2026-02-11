package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/store"
)

// sessionsView renders the session recordings view.
func (m Model) sessionsView() string {
	width := m.width
	if width < 20 {
		width = 80
	}
	contentWidth := width - 4

	if len(m.recordings) == 0 {
		return EmptyStateStyle.Render("No session recordings found. Recordings are captured during agent sessions.")
	}

	summary := lipgloss.NewStyle().Foreground(ColorSubtext0).Render(
		fmt.Sprintf("  %d recorded sessions", len(m.recordings)),
	)

	if m.sessionShowDetail && m.sessionSelectedIdx >= 0 && m.sessionSelectedIdx < len(m.recordings) {
		return lipgloss.JoinVertical(lipgloss.Left,
			summary, "",
			m.renderRecordingDetail(contentWidth),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		summary, "",
		m.renderRecordingsList(contentWidth),
	)
}

func (m Model) renderRecordingsList(width int) string {
	var lines []string

	hdr := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve)
	headerLine := fmt.Sprintf("  %-8s  %-10s  %-14s  %-10s  %s",
		hdr.Render("Session"),
		hdr.Render("Agent"),
		hdr.Render("Start"),
		hdr.Render("Duration"),
		hdr.Render("Events"),
	)
	lines = append(lines, headerLine)
	lines = append(lines, DividerStyle.Render(strings.Repeat("─", width-4)))

	maxVisible := m.height - 14
	if maxVisible < 5 {
		maxVisible = 5
	}

	startIdx := 0
	if m.sessionSelectedIdx >= maxVisible {
		startIdx = m.sessionSelectedIdx - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > len(m.recordings) {
		endIdx = len(m.recordings)
	}

	for i := startIdx; i < endIdx; i++ {
		rec := m.recordings[i]
		isSelected := i == m.sessionSelectedIdx

		agentStyle := lipgloss.NewStyle().Foreground(ColorTeal).Bold(true)
		dateStr := rec.StartTime.Format("01/02 15:04")

		duration := "ongoing"
		if !rec.EndTime.IsZero() {
			d := rec.EndTime.Sub(rec.StartTime)
			mins := int(d.Minutes())
			secs := int(d.Seconds()) % 60
			duration = fmt.Sprintf("%dm %ds", mins, secs)
		}

		eventCount := len(rec.Events)

		line := fmt.Sprintf("  %-8d  %-10s  %-14s  %-10s  %d",
			rec.SessionID,
			agentStyle.Render(rec.Agent),
			lipgloss.NewStyle().Foreground(ColorOverlay0).Render(dateStr),
			lipgloss.NewStyle().Foreground(ColorYellow).Render(duration),
			eventCount,
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

	if len(m.recordings) > maxVisible {
		scrollInfo := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(
			fmt.Sprintf("  showing %d-%d of %d", startIdx+1, endIdx, len(m.recordings)),
		)
		lines = append(lines, "", scrollInfo)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m Model) renderRecordingDetail(width int) string {
	rec := m.recordings[m.sessionSelectedIdx]

	backHint := lipgloss.NewStyle().Foreground(ColorOverlay0).Italic(true).Render("  Press ESC to go back")

	title := DetailTitleStyle.Render(fmt.Sprintf("Session Recording #%d", rec.SessionID))

	agentLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Agent:"),
		lipgloss.NewStyle().Foreground(ColorTeal).Bold(true).Render(rec.Agent))
	startLine := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Started:"),
		rec.StartTime.Format("2006-01-02 15:04:05"))

	var endLine string
	if !rec.EndTime.IsZero() {
		endLine = fmt.Sprintf("%s %s", DetailLabelStyle.Render("Ended:"),
			rec.EndTime.Format("2006-01-02 15:04:05"))
	} else {
		endLine = fmt.Sprintf("%s %s", DetailLabelStyle.Render("Ended:"),
			lipgloss.NewStyle().Foreground(ColorYellow).Render("still running"))
	}

	var durationLine string
	if !rec.EndTime.IsZero() {
		d := rec.EndTime.Sub(rec.StartTime)
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		durationLine = fmt.Sprintf("%s %dm %ds", DetailLabelStyle.Render("Duration:"), mins, secs)
	}

	exitLine := fmt.Sprintf("%s %d", DetailLabelStyle.Render("Exit Code:"), rec.ExitCode)
	exitStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	if rec.ExitCode != 0 {
		exitStyle = lipgloss.NewStyle().Foreground(ColorRed)
	}
	exitLine = fmt.Sprintf("%s %s", DetailLabelStyle.Render("Exit Code:"),
		exitStyle.Render(fmt.Sprintf("%d", rec.ExitCode)))

	eventCountLine := fmt.Sprintf("%s %d", DetailLabelStyle.Render("Events:"), len(rec.Events))

	sections := []string{backHint, "", title, agentLine, startLine, endLine}
	if durationLine != "" {
		sections = append(sections, durationLine)
	}
	sections = append(sections, exitLine, eventCountLine, "")

	// Show event type breakdown
	if len(rec.Events) > 0 {
		sections = append(sections, m.renderEventBreakdown(rec.Events, width))
		sections = append(sections, "")

		// Show recent events (last 20)
		sections = append(sections, m.renderRecentEvents(rec.Events, width))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return CardStyle.Width(width).Render(content)
}

func (m Model) renderEventBreakdown(events []store.RecordingEvent, width int) string {
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.Type]++
	}

	header := DetailLabelStyle.Render("Event Breakdown:")
	var parts []string
	typeColors := map[string]lipgloss.Color{
		"stdout": ColorGreen,
		"stderr": ColorRed,
		"stdin":  ColorBlue,
		"meta":   ColorMauve,
	}

	for t, c := range counts {
		color, ok := typeColors[t]
		if !ok {
			color = ColorText
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(color).Render(
			fmt.Sprintf("%s: %d", t, c),
		))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, "  "+strings.Join(parts, "  "))
}

func (m Model) renderRecentEvents(events []store.RecordingEvent, width int) string {
	header := DetailLabelStyle.Render("Recent Events (last 20):")

	start := 0
	if len(events) > 20 {
		start = len(events) - 20
	}

	maxDataLen := width - 30
	if maxDataLen < 20 {
		maxDataLen = 20
	}

	var lines []string
	lines = append(lines, header)

	typeColors := map[string]lipgloss.Color{
		"stdout": ColorGreen,
		"stderr": ColorRed,
		"stdin":  ColorBlue,
		"meta":   ColorMauve,
	}

	for i := start; i < len(events); i++ {
		e := events[i]
		timeStr := e.Timestamp.Format("15:04:05")

		color, ok := typeColors[e.Type]
		if !ok {
			color = ColorText
		}

		data := strings.ReplaceAll(e.Data, "\n", "\\n")
		data = strings.ReplaceAll(data, "\r", "\\r")
		if len(data) > maxDataLen {
			data = data[:maxDataLen-3] + "..."
		}

		line := fmt.Sprintf("  %s %s %s",
			lipgloss.NewStyle().Foreground(ColorOverlay0).Render(timeStr),
			lipgloss.NewStyle().Foreground(color).Bold(true).Render(fmt.Sprintf("%-6s", e.Type)),
			lipgloss.NewStyle().Foreground(ColorSubtext1).Render(data),
		)
		lines = append(lines, line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
