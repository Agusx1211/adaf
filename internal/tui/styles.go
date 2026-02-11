package tui

import "github.com/charmbracelet/lipgloss"

// Color palette - dark theme inspired by Catppuccin Mocha
var (
	ColorBase     = lipgloss.Color("#1e1e2e")
	ColorSurface0 = lipgloss.Color("#313244")
	ColorSurface1 = lipgloss.Color("#45475a")
	ColorSurface2 = lipgloss.Color("#585b70")
	ColorOverlay0 = lipgloss.Color("#6c7086")
	ColorText     = lipgloss.Color("#cdd6f4")
	ColorSubtext0 = lipgloss.Color("#a6adc8")
	ColorSubtext1 = lipgloss.Color("#bac2de")

	ColorRed     = lipgloss.Color("#f38ba8")
	ColorGreen   = lipgloss.Color("#a6e3a1")
	ColorYellow  = lipgloss.Color("#f9e2af")
	ColorBlue    = lipgloss.Color("#89b4fa")
	ColorMauve   = lipgloss.Color("#cba6f7")
	ColorTeal    = lipgloss.Color("#94e2d5")
	ColorPeach   = lipgloss.Color("#fab387")
	ColorFlamingo = lipgloss.Color("#f2cdcd")
	ColorLavender = lipgloss.Color("#b4befe")
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBase).
			Background(ColorBlue).
			Padding(0, 2).
			MarginBottom(1)

	HeaderProjectName = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorBase)

	HeaderRepoPath = lipgloss.NewStyle().
			Foreground(ColorSurface1).
			Italic(true)
)

// Navigation tab styles
var (
	ActiveTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBase).
			Background(ColorMauve).
			Padding(0, 2)

	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(ColorSubtext0).
				Background(ColorSurface0).
				Padding(0, 2)

	TabBarStyle = lipgloss.NewStyle().
			MarginBottom(1)
)

// Status bar
var (
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext0).
			Background(ColorSurface0).
			Padding(0, 1)

	StatusKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorLavender).
			Background(ColorSurface0)

	StatusValueStyle = lipgloss.NewStyle().
				Foreground(ColorSubtext0).
				Background(ColorSurface0)
)

// Card/panel styles
var (
	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSurface2).
			Padding(1, 2)

	CardTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorLavender).
			MarginBottom(1)

	FocusedCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorMauve).
				Padding(1, 2)
)

// Table styles
var (
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorMauve).
				BorderBottom(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorSurface2).
				Padding(0, 1)

	TableRowStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Padding(0, 1)

	TableSelectedRowStyle = lipgloss.NewStyle().
				Foreground(ColorBase).
				Background(ColorMauve).
				Bold(true).
				Padding(0, 1)
)

// Status indicator styles
var (
	StatusNotStarted = lipgloss.NewStyle().Foreground(ColorOverlay0).SetString("  ")
	StatusInProgress = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).SetString("  ")
	StatusComplete   = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).SetString("  ")
	StatusBlocked    = lipgloss.NewStyle().Foreground(ColorRed).Bold(true).SetString("  ")
)

// Priority styles
var (
	PriorityCritical = lipgloss.NewStyle().Bold(true).Foreground(ColorRed)
	PriorityHigh     = lipgloss.NewStyle().Foreground(ColorPeach)
	PriorityMedium   = lipgloss.NewStyle().Foreground(ColorYellow)
	PriorityLow      = lipgloss.NewStyle().Foreground(ColorSubtext0)
)

// Issue status styles
var (
	IssueOpen       = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)
	IssueInProgress = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
	IssueResolved   = lipgloss.NewStyle().Foreground(ColorSubtext0)
	IssueWontfix    = lipgloss.NewStyle().Foreground(ColorOverlay0)
)

// Progress bar styles
var (
	ProgressBarFilled = lipgloss.NewStyle().Foreground(ColorGreen)
	ProgressBarEmpty  = lipgloss.NewStyle().Foreground(ColorSurface2)
)

// Detail view styles
var (
	DetailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorBlue).
				MarginBottom(1)

	DetailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorMauve).
				Width(16)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(ColorText)

	DetailSectionStyle = lipgloss.NewStyle().
				MarginTop(1).
				MarginBottom(1)

	DetailContentStyle = lipgloss.NewStyle().
				Foreground(ColorSubtext1).
				Padding(0, 1)
)

// List item styles
var (
	ListItemStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			PaddingLeft(2)

	SelectedListItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorMauve).
				PaddingLeft(1).
				SetString("> ")

	ListDimStyle = lipgloss.NewStyle().
			Foreground(ColorOverlay0)
)

// Badge/tag styles
var (
	BadgeStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(ColorBase).
			Background(ColorMauve)
)

// Misc
var (
	DividerStyle = lipgloss.NewStyle().
			Foreground(ColorSurface2)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	EmptyStateStyle = lipgloss.NewStyle().
			Foreground(ColorOverlay0).
			Italic(true).
			Padding(2, 4)

	HelpTextStyle = lipgloss.NewStyle().
			Foreground(ColorSubtext0)
)

// Helper to render a styled status indicator for phase status
func PhaseStatusIndicator(status string) string {
	switch status {
	case "complete":
		return StatusComplete.String()
	case "in_progress":
		return StatusInProgress.String()
	case "blocked":
		return StatusBlocked.String()
	default:
		return StatusNotStarted.String()
	}
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
		return lipgloss.NewStyle().Foreground(ColorText).Render(status)
	}
}
