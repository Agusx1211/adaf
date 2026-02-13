package runtui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/theme"
)

// Panel border styles.
var (
	leftPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorSurface2).
			Padding(1, 1)

	rightPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.ColorSurface2).
			Padding(0, 1)
)

// Header and status bar.
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorBase).
			Background(theme.ColorBlue).
			Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(theme.ColorSubtext0).
			Background(theme.ColorSurface0).
			Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorLavender).
			Background(theme.ColorSurface0)

	statusValueStyle = lipgloss.NewStyle().
				Foreground(theme.ColorSubtext0).
				Background(theme.ColorSurface0)
)

// Left panel section styles.
var (
	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorLavender)

	detailTabActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorBase).
				Background(theme.ColorBlue).
				Padding(0, 1)

	detailTabInactiveStyle = lipgloss.NewStyle().
				Foreground(theme.ColorSubtext0).
				Background(theme.ColorSurface0).
				Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorMauve).
			Width(12)

	valueStyle = lipgloss.NewStyle().
			Foreground(theme.ColorText)

	dimStyle = lipgloss.NewStyle().
			Foreground(theme.ColorOverlay0)
)

// Right panel event styles.
var (
	initLabelStyle = lipgloss.NewStyle().
			Foreground(theme.ColorOverlay0).
			Italic(true)

	thinkingLabelStyle = lipgloss.NewStyle().
				Foreground(theme.ColorOverlay0).
				Italic(true)

	thinkingTextStyle = lipgloss.NewStyle().
				Foreground(theme.ColorOverlay0).
				Italic(true)

	textLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorBlue)

	textStyle = lipgloss.NewStyle().
			Foreground(theme.ColorText)

	toolLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.ColorYellow)

	toolInputStyle = lipgloss.NewStyle().
			Foreground(theme.ColorPeach)

	toolResultStyle = lipgloss.NewStyle().
			Foreground(theme.ColorGreen)

	resultLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(theme.ColorGreen)
)
