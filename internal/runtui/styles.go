package runtui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/tui"
)

// Panel border styles.
var (
	leftPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSurface2).
			Padding(1, 1)

	rightPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorSurface2).
			Padding(0, 1)
)

// Header and status bar.
var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.ColorBase).
			Background(tui.ColorBlue).
			Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(tui.ColorSubtext0).
			Background(tui.ColorSurface0).
			Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.ColorLavender).
			Background(tui.ColorSurface0)

	statusValueStyle = lipgloss.NewStyle().
				Foreground(tui.ColorSubtext0).
				Background(tui.ColorSurface0)
)

// Left panel section styles.
var (
	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(tui.ColorLavender)

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.ColorMauve).
			Width(12)

	valueStyle = lipgloss.NewStyle().
			Foreground(tui.ColorText)

	dimStyle = lipgloss.NewStyle().
			Foreground(tui.ColorOverlay0)
)

// Right panel event styles.
var (
	initLabelStyle = lipgloss.NewStyle().
			Foreground(tui.ColorOverlay0).
			Italic(true)

	thinkingLabelStyle = lipgloss.NewStyle().
				Foreground(tui.ColorOverlay0).
				Italic(true)

	thinkingTextStyle = lipgloss.NewStyle().
				Foreground(tui.ColorOverlay0).
				Italic(true)

	textLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.ColorBlue)

	textStyle = lipgloss.NewStyle().
			Foreground(tui.ColorText)

	toolLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(tui.ColorYellow)

	toolInputStyle = lipgloss.NewStyle().
			Foreground(tui.ColorPeach)

	toolResultStyle = lipgloss.NewStyle().
			Foreground(tui.ColorGreen)

	resultLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(tui.ColorGreen)
)
