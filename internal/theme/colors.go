package theme

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

	ColorRed       = lipgloss.Color("#f38ba8")
	ColorGreen     = lipgloss.Color("#a6e3a1")
	ColorYellow    = lipgloss.Color("#f9e2af")
	ColorBlue      = lipgloss.Color("#89b4fa")
	ColorMauve     = lipgloss.Color("#cba6f7")
	ColorTeal      = lipgloss.Color("#94e2d5")
	ColorPeach     = lipgloss.Color("#fab387")
	ColorFlamingo  = lipgloss.Color("#f2cdcd")
	ColorLavender  = lipgloss.Color("#b4befe")
)

// Status indicator styles
var (
	StatusNotStarted = lipgloss.NewStyle().Foreground(ColorOverlay0).SetString("  ")
	StatusInProgress = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).SetString("  ")
	StatusComplete   = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).SetString("  ")
	StatusBlocked    = lipgloss.NewStyle().Foreground(ColorRed).Bold(true).SetString("  ")
)

// PhaseStatusIndicator returns a styled status indicator for a phase status.
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
