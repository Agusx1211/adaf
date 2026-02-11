package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/store"
)

// dashboardView renders the dashboard content.
func (m Model) dashboardView() string {
	width := m.width
	if width < 20 {
		width = 80
	}

	contentWidth := width - 4 // account for outer padding

	var sections []string

	// -- Project Info Card --
	sections = append(sections, m.renderProjectInfoCard(contentWidth))

	// Layout: left column (plan + current phase) and right column (issues + recent logs)
	leftWidth := contentWidth*55/100 - 2
	rightWidth := contentWidth*45/100 - 2

	if leftWidth < 30 {
		// Narrow terminal: stack vertically
		sections = append(sections, m.renderPlanProgressCard(contentWidth))
		sections = append(sections, m.renderCurrentPhaseCard(contentWidth))
		sections = append(sections, m.renderIssuesSummaryCard(contentWidth))
		sections = append(sections, m.renderRecentLogsCard(contentWidth))
	} else {
		leftCol := lipgloss.JoinVertical(lipgloss.Left,
			m.renderPlanProgressCard(leftWidth),
			m.renderCurrentPhaseCard(leftWidth),
		)
		rightCol := lipgloss.JoinVertical(lipgloss.Left,
			m.renderIssuesSummaryCard(rightWidth),
			m.renderRecentLogsCard(rightWidth),
		)
		columns := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, "  ", rightCol)
		sections = append(sections, columns)
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderProjectInfoCard(width int) string {
	style := CardStyle.Width(width)
	title := CardTitleStyle.Render("Project Overview")

	name := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Name:"), m.project.Name)
	repo := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Repo:"), m.project.RepoPath)
	created := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Created:"), m.project.Created.Format("2006-01-02 15:04"))

	agentCount := len(m.project.AgentConfig)
	agents := fmt.Sprintf("%s %d configured", DetailLabelStyle.Render("Agents:"), agentCount)

	content := lipgloss.JoinVertical(lipgloss.Left, title, name, repo, created, agents)
	return style.Render(content)
}

func (m Model) renderPlanProgressCard(width int) string {
	style := CardStyle.Width(width)
	title := CardTitleStyle.Render("Plan Progress")

	if m.plan == nil || len(m.plan.Phases) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title,
			EmptyStateStyle.Render("No plan defined yet"))
		return style.Render(content)
	}

	total := len(m.plan.Phases)
	complete := 0
	inProgress := 0
	blocked := 0
	for _, p := range m.plan.Phases {
		switch p.Status {
		case "complete":
			complete++
		case "in_progress":
			inProgress++
		case "blocked":
			blocked++
		}
	}

	percentage := 0
	if total > 0 {
		percentage = complete * 100 / total
	}

	// Progress bar
	barWidth := width - 12 // leave room for percentage text
	if barWidth < 10 {
		barWidth = 10
	}
	filledWidth := barWidth * percentage / 100
	emptyWidth := barWidth - filledWidth

	bar := ProgressBarFilled.Render(strings.Repeat("█", filledWidth)) +
		ProgressBarEmpty.Render(strings.Repeat("░", emptyWidth))

	progressLine := fmt.Sprintf("%s %s", bar, lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render(fmt.Sprintf("%d%%", percentage)))

	planTitle := fmt.Sprintf("%s %s", DetailLabelStyle.Render("Plan:"), m.plan.Title)

	stats := fmt.Sprintf(
		"%s  %s  %s  %s",
		lipgloss.NewStyle().Foreground(ColorGreen).Render(fmt.Sprintf("%d done", complete)),
		lipgloss.NewStyle().Foreground(ColorYellow).Render(fmt.Sprintf("%d active", inProgress)),
		lipgloss.NewStyle().Foreground(ColorRed).Render(fmt.Sprintf("%d blocked", blocked)),
		lipgloss.NewStyle().Foreground(ColorOverlay0).Render(fmt.Sprintf("%d total", total)),
	)

	content := lipgloss.JoinVertical(lipgloss.Left, title, planTitle, "", progressLine, stats)
	return style.Render(content)
}

func (m Model) renderCurrentPhaseCard(width int) string {
	style := CardStyle.Width(width)
	title := CardTitleStyle.Render("Current Phase")

	if m.plan == nil || len(m.plan.Phases) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title,
			EmptyStateStyle.Render("No active phase"))
		return style.Render(content)
	}

	// Find first in_progress phase, or first not_started if none active
	var current *store.PlanPhase
	for i := range m.plan.Phases {
		if m.plan.Phases[i].Status == "in_progress" {
			current = &m.plan.Phases[i]
			break
		}
	}
	if current == nil {
		for i := range m.plan.Phases {
			if m.plan.Phases[i].Status == "not_started" {
				current = &m.plan.Phases[i]
				break
			}
		}
	}

	if current == nil {
		content := lipgloss.JoinVertical(lipgloss.Left, title,
			lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render("All phases complete!"))
		return style.Render(content)
	}

	phaseTitle := fmt.Sprintf("%s %s", PhaseStatusIndicator(current.Status), lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(current.Title))

	desc := current.Description
	maxDescLen := width - 6
	if len(desc) > maxDescLen && maxDescLen > 3 {
		desc = desc[:maxDescLen-3] + "..."
	}
	descLine := lipgloss.NewStyle().Foreground(ColorSubtext0).Render(desc)

	content := lipgloss.JoinVertical(lipgloss.Left, title, phaseTitle, descLine)
	return style.Render(content)
}

func (m Model) renderIssuesSummaryCard(width int) string {
	style := CardStyle.Width(width)
	title := CardTitleStyle.Render("Issues Overview")

	if len(m.issues) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title,
			EmptyStateStyle.Render("No issues"))
		return style.Render(content)
	}

	open := 0
	critical := 0
	high := 0
	medium := 0
	low := 0
	for _, issue := range m.issues {
		if issue.Status == "open" || issue.Status == "in_progress" {
			open++
			switch issue.Priority {
			case "critical":
				critical++
			case "high":
				high++
			case "medium":
				medium++
			case "low":
				low++
			}
		}
	}

	totalLine := fmt.Sprintf("%s %s",
		DetailLabelStyle.Render("Open:"),
		lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(fmt.Sprintf("%d", open)),
	)

	breakdown := fmt.Sprintf("  %s  %s  %s  %s",
		PriorityCritical.Render(fmt.Sprintf("%d crit", critical)),
		PriorityHigh.Render(fmt.Sprintf("%d high", high)),
		PriorityMedium.Render(fmt.Sprintf("%d med", medium)),
		PriorityLow.Render(fmt.Sprintf("%d low", low)),
	)

	totalAll := fmt.Sprintf("%s %d",
		DetailLabelStyle.Render("Total:"),
		len(m.issues),
	)

	content := lipgloss.JoinVertical(lipgloss.Left, title, totalLine, breakdown, totalAll)
	return style.Render(content)
}

func (m Model) renderRecentLogsCard(width int) string {
	style := CardStyle.Width(width)
	title := CardTitleStyle.Render("Recent Sessions")

	if len(m.logs) == 0 {
		content := lipgloss.JoinVertical(lipgloss.Left, title,
			EmptyStateStyle.Render("No session logs"))
		return style.Render(content)
	}

	// Show last 5 logs (most recent first)
	start := 0
	if len(m.logs) > 5 {
		start = len(m.logs) - 5
	}

	var lines []string
	lines = append(lines, title)

	for i := len(m.logs) - 1; i >= start; i-- {
		l := m.logs[i]
		agentStyle := lipgloss.NewStyle().Foreground(ColorTeal).Bold(true)
		dateStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
		objStyle := lipgloss.NewStyle().Foreground(ColorText)

		obj := l.Objective
		maxObjLen := width - 28
		if maxObjLen < 10 {
			maxObjLen = 10
		}
		if len(obj) > maxObjLen {
			obj = obj[:maxObjLen-3] + "..."
		}

		line := fmt.Sprintf("  %s %s %s",
			dateStyle.Render(l.Date.Format("01/02")),
			agentStyle.Render(fmt.Sprintf("%-8s", l.Agent)),
			objStyle.Render(obj),
		)
		lines = append(lines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return style.Render(content)
}
