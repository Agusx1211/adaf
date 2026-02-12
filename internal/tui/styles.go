package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/theme"
)

// Re-export colors from theme package for backward compatibility.
var (
	ColorBase     = theme.ColorBase
	ColorSurface0 = theme.ColorSurface0
	ColorSurface1 = theme.ColorSurface1
	ColorSurface2 = theme.ColorSurface2
	ColorOverlay0 = theme.ColorOverlay0
	ColorText     = theme.ColorText
	ColorSubtext0 = theme.ColorSubtext0
	ColorSubtext1 = theme.ColorSubtext1

	ColorRed      = theme.ColorRed
	ColorGreen    = theme.ColorGreen
	ColorYellow   = theme.ColorYellow
	ColorBlue     = theme.ColorBlue
	ColorMauve    = theme.ColorMauve
	ColorTeal     = theme.ColorTeal
	ColorPeach    = theme.ColorPeach
	ColorFlamingo = theme.ColorFlamingo
	ColorLavender = theme.ColorLavender
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorBase).
			Background(theme.ColorBlue).
			Padding(0, 2).
			MarginBottom(1)

	HeaderProjectName = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorBase)

	HeaderRepoPath = lipgloss.NewStyle().
			Foreground(theme.ColorSurface1).
			Italic(true)
)

// Navigation tab styles
var (
	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorBase).
			Background(theme.ColorMauve).
			Padding(0, 2)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(theme.ColorSubtext0).
				Background(theme.ColorSurface0).
				Padding(0, 2)

	TabBarStyle = lipgloss.NewStyle().
			MarginBottom(1)
)

// Status bar
var (
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(theme.ColorSubtext0).
			Background(theme.ColorSurface0).
			Padding(0, 1)

	StatusKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorLavender).
			Background(theme.ColorSurface0)

	StatusValueStyle = lipgloss.NewStyle().
				Foreground(theme.ColorSubtext0).
				Background(theme.ColorSurface0)
)

// Card/panel styles
var (
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorSurface2).
			Padding(1, 2)

	CardTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorLavender).
			MarginBottom(1)

	FocusedCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(theme.ColorMauve).
				Padding(1, 2)
)

// Table styles
var (
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorMauve).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(theme.ColorSurface2).
				Padding(0, 1)

	TableRowStyle = lipgloss.NewStyle().
			Foreground(theme.ColorText).
			Padding(0, 1)

	TableSelectedRowStyle = lipgloss.NewStyle().
				Foreground(theme.ColorBase).
				Background(theme.ColorMauve).
				Bold(true).
				Padding(0, 1)
)

// Status indicator styles
var (
	StatusNotStarted = theme.StatusNotStarted
	StatusInProgress = theme.StatusInProgress
	StatusComplete   = theme.StatusComplete
	StatusBlocked    = theme.StatusBlocked
)

// Priority styles
var (
	PriorityCritical = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorRed)
	PriorityHigh     = lipgloss.NewStyle().Foreground(theme.ColorPeach)
	PriorityMedium   = lipgloss.NewStyle().Foreground(theme.ColorYellow)
	PriorityLow      = lipgloss.NewStyle().Foreground(theme.ColorSubtext0)
)

// Issue status styles
var (
	IssueOpen       = lipgloss.NewStyle().Foreground(theme.ColorGreen).Bold(true)
	IssueInProgress = lipgloss.NewStyle().Foreground(theme.ColorYellow).Bold(true)
	IssueResolved   = lipgloss.NewStyle().Foreground(theme.ColorSubtext0)
	IssueWontfix    = lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
)

// Progress bar styles
var (
	ProgressBarFilled = lipgloss.NewStyle().Foreground(theme.ColorGreen)
	ProgressBarEmpty  = lipgloss.NewStyle().Foreground(theme.ColorSurface2)
)

// Detail view styles
var (
	DetailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorBlue).
				MarginBottom(1)

	DetailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorMauve).
				Width(16)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(theme.ColorText)

	DetailSectionStyle = lipgloss.NewStyle().
				MarginTop(1).
				MarginBottom(1)

	DetailContentStyle = lipgloss.NewStyle().
				Foreground(theme.ColorSubtext1).
				Padding(0, 1)
)

// List item styles
var (
	ListItemStyle = lipgloss.NewStyle().
			Foreground(theme.ColorText).
			PaddingLeft(2)

	SelectedListItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorMauve).
				PaddingLeft(1).
				SetString("> ")

	ListDimStyle = lipgloss.NewStyle().
			Foreground(theme.ColorOverlay0)
)

// Badge/tag styles
var (
	BadgeStyle = lipgloss.NewStyle().
		Padding(0, 1).
		Foreground(theme.ColorBase).
		Background(theme.ColorMauve)
)

// Misc
var (
	DividerStyle = lipgloss.NewStyle().
			Foreground(theme.ColorSurface2)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(theme.ColorRed).
			Bold(true)

	EmptyStateStyle = lipgloss.NewStyle().
			Foreground(theme.ColorOverlay0).
			Italic(true).
			Padding(2, 4)

	HelpTextStyle = lipgloss.NewStyle().
			Foreground(theme.ColorSubtext0)
)

// PhaseStatusIndicator delegates to the theme package.
func PhaseStatusIndicator(status string) string {
	return theme.PhaseStatusIndicator(status)
}

// Helper to render styled priority text
func StyledPriority(priority string) string {
	switch priority {
	case "critical":
		return PriorityCritical.Render("CRITICAL")
	case "high":
		return PriorityHigh.Render("HIGH")
	case "medium":
		return PriorityMedium.Render("MEDIUM")
	case "low":
		return PriorityLow.Render("LOW")
	default:
		return PriorityLow.Render(priority)
	}
}

// Helper to render styled issue status
func StyledIssueStatus(status string) string {
	switch status {
	case "open":
		return IssueOpen.Render("OPEN")
	case "in_progress":
		return IssueInProgress.Render("IN PROGRESS")
	case "resolved":
		return IssueResolved.Render("RESOLVED")
	case "wontfix":
		return IssueWontfix.Render("WONTFIX")
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorText).Render(status)
	}
}
