package cli

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/runtui"
)

func TestAttachModelCtrlDDetaches(t *testing.T) {
	m := attachModel{
		inner: runtui.NewModel("proj", nil, "codex", "", make(chan any, 1), nil),
	}
	m.inner.SetSessionMode(1)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatalf("ctrl+d command = nil, want tea.Quit")
	}
	quitMsg := cmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+d command result = %T, want tea.QuitMsg", quitMsg)
	}

	got, ok := updated.(attachModel)
	if !ok {
		t.Fatalf("updated model type = %T, want attachModel", updated)
	}
	if !got.detached {
		t.Fatalf("detached = false, want true")
	}
}

func TestAttachModelQDetachesWhileRunning(t *testing.T) {
	m := attachModel{
		inner: runtui.NewModel("proj", nil, "codex", "", make(chan any, 1), nil),
	}
	m.inner.SetSessionMode(1)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("q command = nil, want tea.Quit")
	}
	quitMsg := cmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("q command result = %T, want tea.QuitMsg", quitMsg)
	}

	got, ok := updated.(attachModel)
	if !ok {
		t.Fatalf("updated model type = %T, want attachModel", updated)
	}
	if !got.detached {
		t.Fatalf("detached = false, want true")
	}
}

func TestAttachModelQWhenDoneDoesNotMarkDetached(t *testing.T) {
	m := attachModel{
		inner: runtui.NewModel("proj", nil, "codex", "", make(chan any, 1), nil),
	}
	m.inner.SetSessionMode(1)

	updated, _ := m.Update(runtui.AgentLoopDoneMsg{})
	am, ok := updated.(attachModel)
	if !ok {
		t.Fatalf("updated model type = %T, want attachModel", updated)
	}

	updated, cmd := am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatalf("q command when done = nil, want tea.Quit")
	}
	quitMsg := cmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("q command result when done = %T, want tea.QuitMsg", quitMsg)
	}

	got, ok := updated.(attachModel)
	if !ok {
		t.Fatalf("updated model type = %T, want attachModel", updated)
	}
	if got.detached {
		t.Fatalf("detached = true, want false")
	}
}
