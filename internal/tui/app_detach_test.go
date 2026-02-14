package tui

import (
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

func TestStartBackgroundEventDrainConsumesMessages(t *testing.T) {
	ch := make(chan any)
	startBackgroundEventDrain(ch)

	sent := make(chan struct{})
	go func() {
		ch <- runtui.AgentRawOutputMsg{Data: "hello"}
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatalf("send blocked; background drain not consuming events")
	}

	close(ch)
}

func TestUpdateRunningDetachDrainsEventChannel(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	ch := make(chan any)
	m := AppModel{
		store:      s,
		state:      stateRunning,
		runModel:   runtui.NewModel("proj", nil, "", "", make(chan any, 1), nil),
		runEventCh: ch,
		runCancel:  func() {},
		sessionID:  9,
	}

	updated, _ := m.updateRunning(runtui.DetachMsg{SessionID: 9})
	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if got.runEventCh != nil {
		t.Fatalf("runEventCh = %v, want nil", got.runEventCh)
	}

	// The original channel should still be drainable (background goroutine consuming).
	sent := make(chan struct{})
	go func() {
		ch <- runtui.AgentRawOutputMsg{Data: "after-detach"}
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatalf("send blocked after detach; expected background drain")
	}

	close(ch)
}

func TestUpdateRunningDetachReturnsToSelector(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	m := AppModel{
		store:      s,
		state:      stateRunning,
		runModel:   runtui.NewModel("proj", nil, "", "", make(chan any, 1), nil),
		runEventCh: make(chan any, 1),
		runCancel:  func() {},
		sessionID:  9,
	}

	updated, _ := m.updateRunning(runtui.DetachMsg{SessionID: 9})
	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if got.state != stateSelector {
		t.Fatalf("state = %v, want stateSelector", got.state)
	}
	if got.runEventCh != nil {
		t.Fatalf("runEventCh = %v, want nil", got.runEventCh)
	}
	if got.runCancel != nil {
		t.Fatalf("runCancel = %v, want nil", got.runCancel)
	}
	if got.sessionID != 0 {
		t.Fatalf("sessionID = %d, want 0", got.sessionID)
	}
}

func TestUpdateRunningBackToSelectorCancels(t *testing.T) {
	tmp := t.TempDir()
	s, err := store.New(tmp)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: tmp}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	cancelled := false
	m := AppModel{
		store:      s,
		state:      stateRunning,
		runModel:   runtui.NewModel("proj", nil, "", "", make(chan any, 1), nil),
		runEventCh: make(chan any, 1),
		runCancel: func() {
			cancelled = true
		},
		sessionID: 7,
	}

	updated, _ := m.updateRunning(runtui.BackToSelectorMsg{})
	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if !cancelled {
		t.Fatal("runCancel was not called")
	}
	if got.state != stateSelector {
		t.Fatalf("state = %v, want stateSelector", got.state)
	}
}

func TestStartLoopSessionFailureSetsSelectorMessage(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-dir")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = file.Close()

	s, err := store.New(file.Name())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	m := AppModel{
		store:     s,
		state:     stateSelector,
		globalCfg: &config.GlobalConfig{},
	}

	updated, _ := m.startLoopSession(
		config.LoopDef{Name: "loop", Steps: []config.LoopStep{{Profile: "p1", Turns: 1}}},
		[]config.Profile{{Name: "p1", Agent: "generic"}},
		"p1",
		"generic",
		nil,
		1,
	)
	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if got.selector.Msg == "" {
		t.Fatal("selectorMsg = empty, want non-empty start failure message")
	}
	if got.state != stateSelector {
		t.Fatalf("state = %v, want stateSelector", got.state)
	}
}

func TestShowSessionsMultipleOpensPicker(t *testing.T) {
	t.Setenv("ADAF_TURN_ID", "")
	t.Setenv("ADAF_SESSION_ID", "")
	t.Setenv("ADAF_AGENT", "")
	t.Setenv("HOME", t.TempDir())

	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	m := NewApp(s)
	_ = createRunningSessionMeta(t, projectDir, "p1", "generic")
	_ = createRunningSessionMeta(t, projectDir, "p2", "generic")

	updated, _ := m.showSessions()
	got, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("updated model type = %T, want tui.AppModel", updated)
	}
	if got.state != stateSessionPicker {
		t.Fatalf("state = %v, want stateSessionPicker", got.state)
	}
	if len(got.activeSessions) < 2 {
		t.Fatalf("activeSessions = %d, want >=2", len(got.activeSessions))
	}
}

func TestUpdateSessionPickerNavigationAndCancel(t *testing.T) {
	m := AppModel{
		state: stateSessionPicker,
		activeSessions: []session.SessionMeta{
			{ID: 1, ProfileName: "a", AgentName: "generic", Status: "running"},
			{ID: 2, ProfileName: "b", AgentName: "generic", Status: "running"},
		},
		selector: SelectorState{
			SessionPickSel: 0,
		},
	}

	updated, _ := m.updateSessionPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got := updated.(AppModel)
	if got.selector.SessionPickSel != 1 {
		t.Fatalf("sessionPickSel after j = %d, want 1", got.selector.SessionPickSel)
	}

	updated, _ = got.updateSessionPicker(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	got = updated.(AppModel)
	if got.selector.SessionPickSel != 0 {
		t.Fatalf("sessionPickSel after k = %d, want 0", got.selector.SessionPickSel)
	}

	updated, _ = got.updateSessionPicker(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(AppModel)
	if got.state != stateSelector {
		t.Fatalf("state after esc = %v, want stateSelector", got.state)
	}
}

func createRunningSessionMeta(t *testing.T, projectDir, profileName, agentName string) int {
	t.Helper()

	id, err := session.CreateSession(session.DaemonConfig{
		ProjectDir:  projectDir,
		ProjectName: "proj",
		WorkDir:     projectDir,
		ProfileName: profileName,
		AgentName:   agentName,
		Loop: config.LoopDef{
			Name: "test-loop",
			Steps: []config.LoopStep{
				{Profile: profileName, Turns: 1},
			},
		},
		Profiles: []config.Profile{
			{Name: profileName, Agent: agentName},
		},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	meta, err := session.LoadMeta(id)
	if err != nil {
		t.Fatalf("LoadMeta(%d): %v", id, err)
	}
	meta.Status = "running"
	meta.PID = os.Getpid()
	if err := session.SaveMeta(id, meta); err != nil {
		t.Fatalf("SaveMeta(%d): %v", id, err)
	}
	return id
}
