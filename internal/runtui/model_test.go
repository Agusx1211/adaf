package runtui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

func TestWrapRenderableLinesWordWrap(t *testing.T) {
	lines := wrapRenderableLines([]string{"one two three four"}, 7)
	if len(lines) != 3 {
		t.Fatalf("wrapped line count = %d, want 3", len(lines))
	}

	got := make([]string, 0, len(lines))
	for _, line := range lines {
		got = append(got, ansi.Strip(line))
	}

	want := []string{"one two", "three", "four"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDetailLinesWrapsWithoutTruncating(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any), nil)
	m.SetSize(96, 20)
	width := m.rcWidth()
	long := strings.Repeat("x", width+17)
	m.addLine(long)

	lines := m.detailLines(width)
	if len(lines) < 2 {
		t.Fatalf("wrapped line count = %d, want >= 2", len(lines))
	}

	var combined strings.Builder
	for i, line := range lines {
		plain := ansi.Strip(line)
		if lipgloss.Width(plain) > width {
			t.Fatalf("line[%d] width = %d, want <= %d", i, lipgloss.Width(plain), width)
		}
		combined.WriteString(plain)
	}

	if combined.String() != long {
		t.Errorf("wrapped text mismatch: got len=%d want len=%d", len(combined.String()), len(long))
	}
}

func TestDetachMessageQuitsInSessionMode(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.SetSessionMode(7)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatalf("ctrl+d command = nil, want detach command")
	}
	detachMsg := cmd()
	detach, ok := detachMsg.(DetachMsg)
	if !ok {
		t.Fatalf("ctrl+d msg type = %T, want DetachMsg", detachMsg)
	}
	if detach.SessionID != 7 {
		t.Fatalf("detach session id = %d, want 7", detach.SessionID)
	}

	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	_, quitCmd := m2.Update(detach)
	if quitCmd == nil {
		t.Fatalf("quit command = nil, want tea.Quit")
	}
	quitMsg := quitCmd()
	if _, ok := quitMsg.(tea.QuitMsg); !ok {
		t.Fatalf("quit command result type = %T, want tea.QuitMsg", quitMsg)
	}
}

func TestTurnTerminologyInRunPanel(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	updated, _ := m.Update(AgentStartedMsg{SessionID: 42})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	if len(m2.lines) == 0 {
		t.Fatal("expected start line to be recorded")
	}
	last := ansi.Strip(m2.lines[len(m2.lines)-1].text)
	if !strings.Contains(last, "Turn #42 started") {
		t.Fatalf("start line = %q, want to contain %q", last, "Turn #42 started")
	}

	updated, _ = m2.Update(AgentFinishedMsg{
		SessionID: 42,
		Result: &agent.Result{
			ExitCode: 0,
			Duration: time.Second,
		},
	})
	m3, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	foundFinish := false
	for _, line := range m3.lines {
		if strings.Contains(ansi.Strip(line.text), "Turn #42 finished") {
			foundFinish = true
			break
		}
	}
	if !foundFinish {
		t.Fatal("expected finish line to use Turn terminology")
	}

	entries := m3.commandEntries()
	foundTurnEntry := false
	for _, entry := range entries {
		if strings.Contains(entry.title, "turn #42") {
			foundTurnEntry = true
			break
		}
	}
	if !foundTurnEntry {
		t.Fatal("expected command center entry title to use turn #")
	}

	prefix := m3.scopePrefix("session:42")
	if !strings.Contains(prefix, "turn #42") {
		t.Fatalf("scope prefix = %q, want to contain %q", prefix, "turn #42")
	}
}

func TestWaitForSpawnsTurnStatus(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	updated, _ := m.Update(AgentStartedMsg{SessionID: 42})
	m1, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	updated, _ = m1.Update(AgentFinishedMsg{
		SessionID:     42,
		WaitForSpawns: true,
		Result: &agent.Result{
			ExitCode: 0,
			Duration: 2 * time.Second,
		},
	})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	s := m2.sessions[42]
	if s == nil {
		t.Fatal("expected session #42 to exist")
	}
	if s.Status != "waiting_for_spawns" {
		t.Fatalf("status = %q, want %q", s.Status, "waiting_for_spawns")
	}

	entries := m2.commandEntries()
	foundWaiting := false
	for _, entry := range entries {
		if entry.scope == "session:42" && entry.status == "waiting_for_spawns" {
			foundWaiting = true
			break
		}
	}
	if !foundWaiting {
		t.Fatal("expected command entries to show waiting_for_spawns status")
	}

	updated, _ = m2.Update(AgentStartedMsg{SessionID: 43})
	m3, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	if got := m3.sessions[42]; got == nil || got.Status != "completed" {
		t.Fatalf("session #42 status = %v, want completed after next turn starts", got)
	}
}

func TestWaitForSpawnsResumeSameTurn(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	updated, _ := m.Update(AgentStartedMsg{SessionID: 42})
	m1 := updated.(Model)

	updated, _ = m1.Update(AgentFinishedMsg{
		SessionID:     42,
		WaitForSpawns: true,
		Result: &agent.Result{
			ExitCode: 0,
			Duration: time.Second,
		},
	})
	m2 := updated.(Model)

	updated, _ = m2.Update(AgentStartedMsg{SessionID: 42})
	m3 := updated.(Model)

	if len(m3.sessionOrder) != 1 {
		t.Fatalf("session entries = %d, want 1", len(m3.sessionOrder))
	}
	s := m3.sessions[42]
	if s == nil {
		t.Fatal("expected session #42 to exist")
	}
	if s.Status != "running" {
		t.Fatalf("status = %q, want %q", s.Status, "running")
	}
	if s.Action != "resumed" {
		t.Fatalf("action = %q, want %q", s.Action, "resumed")
	}
	if !s.EndedAt.IsZero() {
		t.Fatalf("ended_at should be reset on resume, got %v", s.EndedAt)
	}

	foundResumed := false
	for _, line := range m3.lines {
		if strings.Contains(ansi.Strip(line.text), "Turn #42 resumed") {
			foundResumed = true
			break
		}
	}
	if !foundResumed {
		t.Fatal("expected resume line to be rendered for same turn resume")
	}
}

func TestCommandEntriesHierarchyAndHistory(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	now := time.Now().Add(-10 * time.Minute)
	m.sessions = map[int]*sessionStatus{
		1: {ID: 1, Agent: "codex", Status: "completed", StartedAt: now, EndedAt: now.Add(5 * time.Second)},
		2: {ID: 2, Agent: "codex", Status: "completed", StartedAt: now, EndedAt: now.Add(6 * time.Second)},
		3: {ID: 3, Agent: "codex", Status: "completed", StartedAt: now, EndedAt: now.Add(7 * time.Second)},
		4: {ID: 4, Agent: "codex", Status: "completed", StartedAt: now, EndedAt: now.Add(8 * time.Second)},
		5: {ID: 5, Agent: "codex", Status: "completed", StartedAt: now, EndedAt: now.Add(9 * time.Second)},
		6: {ID: 6, Agent: "codex", Status: "running", StartedAt: now},
	}
	m.sessionOrder = []int{1, 2, 3, 4, 5, 6}
	m.spawns = []SpawnInfo{
		{ID: 10, ParentTurnID: 6, ChildTurnID: 60, Profile: "reviewer", Role: "qa", Status: "running"},
		{ID: 11, ParentTurnID: 60, ParentSpawnID: 10, ChildTurnID: 61, Profile: "writer", Role: "documentator", Status: "queued"},
	}
	m.spawnFirstSeen[10] = time.Now().Add(-30 * time.Second)
	m.spawnFirstSeen[11] = time.Now().Add(-20 * time.Second)

	entries := m.commandEntries()
	completedCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.scope, "session:") && entry.status == "completed" {
			completedCount++
		}
	}
	if completedCount != 5 {
		t.Fatalf("completed entries = %d, want 5", completedCount)
	}

	parentIdx, childIdx, nestedIdx := -1, -1, -1
	for i, entry := range entries {
		if entry.scope == "session:6" {
			parentIdx = i
		}
		if entry.scope == "spawn:10" {
			childIdx = i
			if entry.depth != 1 {
				t.Fatalf("spawn entry depth = %d, want 1", entry.depth)
			}
		}
		if entry.scope == "spawn:11" {
			nestedIdx = i
			if entry.depth != 2 {
				t.Fatalf("nested spawn entry depth = %d, want 2", entry.depth)
			}
		}
	}
	if parentIdx == -1 || childIdx == -1 || nestedIdx == -1 {
		t.Fatalf("missing hierarchy entries: parent=%d child=%d nested=%d", parentIdx, childIdx, nestedIdx)
	}
	if childIdx <= parentIdx {
		t.Fatalf("spawn entry index = %d, want greater than parent index %d", childIdx, parentIdx)
	}
	if nestedIdx <= childIdx {
		t.Fatalf("nested spawn entry index = %d, want greater than parent spawn index %d", nestedIdx, childIdx)
	}

	var lines []string
	_ = m.appendAgentsList(&lines, 80, entries)
	rendered := strings.Join(stripStyledLines(lines), "\n")
	if !strings.Contains(rendered, "+- #10 reviewer as qa") {
		t.Fatalf("agents list missing spawn role/title; output:\n%s", rendered)
	}
	if !strings.Contains(rendered, "|  +- #11 writer as documentator") {
		t.Fatalf("agents list missing nested hierarchy prefix; output:\n%s", rendered)
	}
}

func TestIssueAndDocDetailModes(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.issues = []store.Issue{
		{
			ID:          7,
			Title:       "Fix flaky test",
			Description: "Repro:\n- run go test ./...",
			Status:      "open",
			Priority:    "high",
			Created:     time.Now().Add(-2 * time.Hour),
			Updated:     time.Now().Add(-time.Hour),
		},
	}
	m.docs = []store.Doc{
		{
			ID:      "arch",
			Title:   "Architecture",
			Content: "System overview\nComponent map",
			Created: time.Now().Add(-3 * time.Hour),
			Updated: time.Now().Add(-30 * time.Minute),
		},
	}

	m.leftSection = leftSectionIssues
	issueLines := strings.Join(stripStyledLines(m.detailLines(80)), "\n")
	if !strings.Contains(issueLines, "Issue #7") {
		t.Fatalf("issue detail did not render selected issue, got: %q", issueLines)
	}
	if !strings.Contains(issueLines, "Fix flaky test") {
		t.Fatalf("issue detail did not include title, got: %q", issueLines)
	}

	m.leftSection = leftSectionDocs
	docLines := strings.Join(stripStyledLines(m.detailLines(80)), "\n")
	if !strings.Contains(docLines, "Doc arch") {
		t.Fatalf("doc detail did not render selected doc, got: %q", docLines)
	}
	if !strings.Contains(docLines, "System overview") {
		t.Fatalf("doc detail did not include content, got: %q", docLines)
	}
}

func TestIssueSelectionWithKeyboard(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.focus = focusCommand
	m.leftSection = leftSectionIssues
	m.issues = []store.Issue{
		{ID: 1, Title: "one", Status: "open", Priority: "low"},
		{ID: 2, Title: "two", Status: "open", Priority: "low"},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	if got.selectedIssue != 1 {
		t.Fatalf("selectedIssue = %d, want 1", got.selectedIssue)
	}
	if got.selectedScope() != "" {
		t.Fatalf("selectedScope = %q, want empty in issue mode", got.selectedScope())
	}
}

func TestPlanAndLogDetailModes(t *testing.T) {
	m := NewModel("proj", &store.Plan{
		ID:     "p1",
		Title:  "Roadmap",
		Status: "active",
		Phases: []store.PlanPhase{
			{ID: "phase-a", Title: "Plan", Status: "in_progress", Priority: 1, Description: "Build plan"},
			{ID: "phase-b", Title: "Ship", Status: "not_started", Priority: 2, Description: "Ship feature"},
		},
	}, "codex", "", make(chan any, 1), nil)
	m.leftSection = leftSectionPlan
	m.selectedPhase = 1

	planLines := strings.Join(stripStyledLines(m.detailLines(80)), "\n")
	if !strings.Contains(planLines, "Phase 2/2") {
		t.Fatalf("plan detail did not render selected phase, got: %q", planLines)
	}
	if !strings.Contains(planLines, "Ship") {
		t.Fatalf("plan detail did not include selected phase title, got: %q", planLines)
	}

	m.leftSection = leftSectionLogs
	m.turns = []store.Turn{
		{
			ID:           12,
			Date:         time.Now().Add(-time.Minute),
			Agent:        "codex",
			ProfileName:  "reviewer",
			Objective:    "Audit changes",
			WhatWasBuilt: "Added tests",
			CurrentState: "All green",
			NextSteps:    "Merge",
			BuildState:   "passing",
		},
	}

	logLines := strings.Join(stripStyledLines(m.detailLines(80)), "\n")
	if !strings.Contains(logLines, "Turn #12") {
		t.Fatalf("log detail did not render selected turn, got: %q", logLines)
	}
	if !strings.Contains(logLines, "Audit changes") {
		t.Fatalf("log detail did not include objective, got: %q", logLines)
	}
}

func TestStreamingDeltaAfterModelCopyDoesNotPanic(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	// Bubble Tea app models are value types; this simulates parent-model copying.
	copied := m

	start := AgentEventMsg{
		Event: stream.ClaudeEvent{
			Type:         "content_block_start",
			ContentBlock: &stream.ContentBlock{Type: "text"},
		},
	}
	delta := AgentEventMsg{
		Event: stream.ClaudeEvent{
			Type:  "content_block_delta",
			Delta: &stream.Delta{Text: "hello"},
		},
	}

	updated, _ := copied.Update(start)
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	updated, _ = m2.Update(delta)
	m3, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic after model copy: %v", r)
		}
	}()

	updated, _ = m3.Update(delta)
	m4, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}
	if m4.streamBuf == nil || m4.streamBuf.Len() == 0 {
		t.Fatal("expected streaming buffer to contain accumulated delta text")
	}
}

func TestRawOutputRoutesToSpawnScope(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	updated, _ := m.Update(SpawnStatusMsg{
		Spawns: []SpawnInfo{
			{ID: 9, Profile: "devstral2", Status: "running"},
		},
	})
	m1, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	updated, _ = m1.Update(AgentRawOutputMsg{
		SessionID: -9,
		Data:      "hello from spawn\n",
	})
	m2, ok := updated.(Model)
	if !ok {
		t.Fatalf("updated model type = %T, want runtui.Model", updated)
	}

	found := false
	for _, line := range m2.lines {
		if strings.Contains(ansi.Strip(line.text), "hello from spawn") {
			found = true
			if line.scope != "spawn:9" {
				t.Fatalf("line scope = %q, want %q", line.scope, "spawn:9")
			}
		}
	}
	if !found {
		t.Fatal("expected spawn output line to be rendered")
	}
}

func TestDetailLayerCycleKeys(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)
	m.focus = focusDetail
	m.leftSection = leftSectionAgents

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m1 := updated.(Model)
	if m1.detailLayer != detailLayerSimplified {
		t.Fatalf("detailLayer after 't' = %v, want %v", m1.detailLayer, detailLayerSimplified)
	}

	updated, _ = m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	m2 := updated.(Model)
	if m2.detailLayer != detailLayerRaw {
		t.Fatalf("detailLayer after 'T' = %v, want %v", m2.detailLayer, detailLayerRaw)
	}
}

func TestPromptLayerShowsCapturedPrompt(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	updated, _ := m.Update(AgentStartedMsg{SessionID: 5, TurnHexID: "turnhex"})
	m1 := updated.(Model)
	updated, _ = m1.Update(AgentPromptMsg{
		SessionID:      5,
		TurnHexID:      "turnhex",
		Prompt:         "line one\nline two",
		IsResume:       true,
		Truncated:      true,
		OriginalLength: 9999,
	})
	m2 := updated.(Model)
	m2.detailLayer = detailLayerPrompt

	rendered := strings.Join(stripStyledLines(m2.detailLines(80)), "\n")
	if !strings.Contains(rendered, "line one") || !strings.Contains(rendered, "line two") {
		t.Fatalf("prompt detail missing prompt body:\n%s", rendered)
	}
	if !strings.Contains(rendered, "resume prompt") {
		t.Fatalf("prompt detail missing metadata:\n%s", rendered)
	}
}

func TestLastMessageLayerShowsFinishedAssistantText(t *testing.T) {
	m := NewModel("proj", nil, "codex", "", make(chan any, 1), nil)

	updated, _ := m.Update(AgentStartedMsg{SessionID: 9, TurnHexID: "abc"})
	m1 := updated.(Model)
	updated, _ = m1.Update(AgentEventMsg{
		Event: stream.ClaudeEvent{
			Type: "assistant",
			AssistantMessage: &stream.AssistantMessage{
				Role: "assistant",
				Content: []stream.ContentBlock{
					{Type: "text", Text: "Final answer payload"},
				},
			},
		},
	})
	m2 := updated.(Model)
	updated, _ = m2.Update(AgentFinishedMsg{
		SessionID: 9,
		TurnHexID: "abc",
		Result: &agent.Result{
			ExitCode: 0,
			Duration: time.Second,
		},
	})
	m3 := updated.(Model)
	m3.detailLayer = detailLayerLastMessage

	rendered := strings.Join(stripStyledLines(m3.detailLines(80)), "\n")
	if !strings.Contains(rendered, "Final answer payload") {
		t.Fatalf("last-message layer missing assistant text:\n%s", rendered)
	}
}

func stripStyledLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, ansi.Strip(line))
	}
	return out
}
