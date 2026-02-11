package tui

import "github.com/charmbracelet/bubbles/key"

// ViewKeyMap defines the global key bindings for navigating views.
type ViewKeyMap struct {
	Quit      key.Binding
	Tab       key.Binding
	ShiftTab  key.Binding
	View1     key.Binding
	View2     key.Binding
	View3     key.Binding
	View4     key.Binding
	View5     key.Binding
	View6     key.Binding
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Escape    key.Binding
	Help      key.Binding
}

// DefaultKeyMap returns the default key map for the application.
func DefaultKeyMap() ViewKeyMap {
	return ViewKeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next view"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev view"),
		),
		View1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "dashboard"),
		),
		View2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "plan"),
		),
		View3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "issues"),
		),
		View4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "logs"),
		),
		View5: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "sessions"),
		),
		View6: key.NewBinding(
			key.WithKeys("6"),
			key.WithHelp("6", "docs"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("k/up", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("j/down", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
	}
}
