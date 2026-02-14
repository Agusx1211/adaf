package runtui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
	"github.com/agusx1211/adaf/internal/theme"
)

const (
	leftPanelOuterWidth = 44
)

type paneFocus int

const (
	focusDetail paneFocus = iota
	focusCommand
)

type leftPanelSection int

const (
	leftSectionAgents leftPanelSection = iota
	leftSectionIssues
	leftSectionDocs
	leftSectionPlan
	leftSectionLogs
)

type scopedLine struct {
	scope string
	text  string
}

type sessionStatus struct {
	ID         int
	Agent      string
	Profile    string
	Model      string
	Status     string
	Action     string
	StartedAt  time.Time
	EndedAt    time.Time
	LastUpdate time.Time
}

type commandEntry struct {
	scope    string
	title    string
	status   string
	action   string
	duration string
	depth    int
}

// Model is the bubbletea model for the adaf run TUI.
type Model struct {
	width  int
	height int

	// Project/plan data.
	projectName  string
	plan         *store.Plan
	projectStore *store.Store
	issues       []store.Issue
	docs         []store.Doc
	activeLoop   *store.LoopRun
	lastDataLoad time.Time

	// Agent info.
	agentName string
	modelName string
	sessionID int
	startTime time.Time
	elapsed   time.Duration

	// Usage stats (populated from stream events).
	inputTokens  int
	outputTokens int
	costUSD      float64
	numTurns     int

	// Right panel: completed styled lines and scroll position.
	lines      []scopedLine
	scrollPos  int
	autoScroll bool
	focus      paneFocus
	// Current detail layer for the right panel in agent view.
	detailLayer detailLayer
	// Derived layer content and metrics.
	simplifiedLines []scopedLine
	activityLines   []scopedLine
	statsByScope    map[string]*detailStats

	// Streaming delta accumulator. Use pointers so Model value copies do not
	// trip strings.Builder copy checks in Bubble Tea update cycles.
	streamBuf    *strings.Builder
	currentScope string

	// Tool input accumulator: collects partial_json deltas for parsing on block stop.
	toolInputBuf *strings.Builder

	// Current streaming block state.
	currentBlockType string
	currentToolName  string

	// Hierarchy: active spawns for this session.
	spawns []SpawnInfo
	// Track first-seen and last-seen status for spawn entries.
	spawnFirstSeen map[int]time.Time
	spawnStatus    map[int]string

	// Loop state.
	loopName        string
	loopCycle       int
	loopStep        int
	loopTotalSteps  int
	loopStepProfile string
	loopRunHexID    string
	loopStepHexID   string

	// Event channel and lifecycle state.
	eventCh    chan any
	cancelFunc context.CancelFunc
	done       bool
	stopping   bool
	exitErr    error

	// Session mode: when non-zero, this model is attached to a session daemon
	// and supports detach (Ctrl+D).
	sessionModeID int
	live          bool

	// Command center state.
	sessions      map[int]*sessionStatus
	sessionOrder  []int
	selectedEntry int
	leftSection   leftPanelSection
	selectedIssue int
	selectedDoc   int
	selectedPhase int
	turns         []store.Turn
	selectedTurn  int

	// Raw output accumulators per scope for line-based rendering.
	rawRemainder map[string]string

	// Stderr deduplication state per scope.
	lastStderrByScope map[string]string
	stderrRepeatCount map[string]int

	// Prompt and final-message snapshots by scope.
	promptsByScope           map[string]promptSnapshot
	latestPromptScope        string
	lastMessageByScope       map[string]string
	assistantMessagesByScope map[string][]string
	finalMessageByScope      map[string]finalMessageSnapshot
	finalizedTurnHexByScope  map[string]string
	latestFinalScope         string

	// Store-change activity tracking snapshots.
	activityBaselineReady bool
	activityLoopRunID     int
	knownIssues           map[int]store.Issue
	knownDocs             map[string]store.Doc
	knownNotes            map[int]struct{}
	knownLoopMessages     map[string]struct{}
	knownSpawns           map[int]store.SpawnRecord
	knownSpawnMessages    map[string]struct{}
}

// NewModel creates a new Model with the given configuration.
func NewModel(projectName string, plan *store.Plan, agentName, modelName string, eventCh chan any, cancel context.CancelFunc) Model {
	return Model{
		projectName:              projectName,
		plan:                     plan,
		agentName:                agentName,
		modelName:                modelName,
		startTime:                time.Now(),
		autoScroll:               true,
		focus:                    focusDetail,
		detailLayer:              detailLayerRaw,
		eventCh:                  eventCh,
		cancelFunc:               cancel,
		streamBuf:                &strings.Builder{},
		toolInputBuf:             &strings.Builder{},
		sessions:                 make(map[int]*sessionStatus),
		spawnFirstSeen:           make(map[int]time.Time),
		spawnStatus:              make(map[int]string),
		rawRemainder:             make(map[string]string),
		lastStderrByScope:        make(map[string]string),
		stderrRepeatCount:        make(map[string]int),
		statsByScope:             make(map[string]*detailStats),
		promptsByScope:           make(map[string]promptSnapshot),
		lastMessageByScope:       make(map[string]string),
		assistantMessagesByScope: make(map[string][]string),
		finalMessageByScope:      make(map[string]finalMessageSnapshot),
		finalizedTurnHexByScope:  make(map[string]string),
		knownIssues:              make(map[int]store.Issue),
		knownDocs:                make(map[string]store.Doc),
		knownNotes:               make(map[int]struct{}),
		knownLoopMessages:        make(map[string]struct{}),
		knownSpawns:              make(map[int]store.SpawnRecord),
		knownSpawnMessages:       make(map[string]struct{}),
		leftSection:              leftSectionAgents,
		live:                     true,
	}
}

// SetSize sets the terminal dimensions on the model. This is used when
// embedding the Model inside a parent so the first render has correct sizing.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// SetStore attaches a project store so the run TUI can surface issues/docs.
func (m *Model) SetStore(s *store.Store) {
	m.projectStore = s
	m.reloadProjectData()
}

func (m *Model) reloadProjectData() {
	if m.projectStore == nil {
		return
	}
	planID := ""
	if m.plan != nil {
		status := strings.TrimSpace(m.plan.Status)
		if status == "" || status == "active" {
			planID = strings.TrimSpace(m.plan.ID)
		}
	}
	if planID != "" {
		if issues, err := m.projectStore.ListIssuesForPlan(planID); err == nil {
			m.issues = issues
		}
		if docs, err := m.projectStore.ListDocsForPlan(planID); err == nil {
			m.docs = docs
		}
	} else {
		if issues, err := m.projectStore.ListSharedIssues(); err == nil {
			m.issues = issues
		}
		if docs, err := m.projectStore.ListSharedDocs(); err == nil {
			m.docs = docs
		}
	}
	if run, err := m.projectStore.ActiveLoopRun(); err == nil {
		m.activeLoop = run
	}
	if turns, err := m.projectStore.ListTurns(); err == nil {
		// Show newest turns first in Logs view.
		for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
			turns[i], turns[j] = turns[j], turns[i]
		}
		m.turns = turns
	}
	if len(m.issues) == 0 {
		m.selectedIssue = 0
	} else if m.selectedIssue >= len(m.issues) {
		m.selectedIssue = len(m.issues) - 1
	}
	if len(m.docs) == 0 {
		m.selectedDoc = 0
	} else if m.selectedDoc >= len(m.docs) {
		m.selectedDoc = len(m.docs) - 1
	}
	if m.plan == nil || len(m.plan.Phases) == 0 {
		m.selectedPhase = 0
	} else if m.selectedPhase >= len(m.plan.Phases) {
		m.selectedPhase = len(m.plan.Phases) - 1
	}
	if len(m.turns) == 0 {
		m.selectedTurn = 0
	} else if m.selectedTurn >= len(m.turns) {
		m.selectedTurn = len(m.turns) - 1
	}
	m.refreshStoreActivity()
	m.lastDataLoad = time.Now()
}

// SetLoopInfo configures loop display information on the model.
func (m *Model) SetLoopInfo(name string, totalSteps int) {
	m.loopName = name
	m.loopTotalSteps = totalSteps
}

// SetSessionMode marks this model as attached to a session daemon.
// When in session mode, Ctrl+D detaches instead of scrolling.
func (m *Model) SetSessionMode(sessionID int) {
	m.sessionModeID = sessionID
	m.live = false
}

// SessionMode returns the session ID if in session mode, or 0.
func (m Model) SessionMode() int {
	return m.sessionModeID
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.eventCh),
		tickEvery(),
		tea.SetWindowTitle("adaf run"),
	)
}

// waitForEvent returns a Cmd that waits for the next event on the channel.
func waitForEvent(ch chan any) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return AgentLoopDoneMsg{}
		}
		return msg
	}
}

// tickEvery returns a Cmd that sends a tickMsg after 1 second.
func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case SessionSnapshotMsg:
		if msg.Loop.TotalSteps > 0 {
			m.loopTotalSteps = msg.Loop.TotalSteps
		}
		m.loopCycle = msg.Loop.Cycle
		m.loopStep = msg.Loop.StepIndex
		if msg.Loop.Profile != "" {
			m.loopStepProfile = msg.Loop.Profile
		}
		if msg.Loop.RunHexID != "" {
			m.loopRunHexID = msg.Loop.RunHexID
		}
		if msg.Loop.StepHexID != "" {
			m.loopStepHexID = msg.Loop.StepHexID
		}
		if msg.Session != nil && msg.Session.SessionID > 0 {
			m.sessionID = msg.Session.SessionID
			s := m.ensureSession(msg.Session.SessionID)
			if s != nil {
				if msg.Session.Agent != "" {
					s.Agent = msg.Session.Agent
					m.agentName = msg.Session.Agent
				}
				if s.Agent == "" {
					s.Agent = m.agentName
				}
				if msg.Session.Profile != "" {
					s.Profile = msg.Session.Profile
				}
				if s.Profile == "" {
					s.Profile = m.loopStepProfile
				}
				if msg.Session.Model != "" {
					s.Model = msg.Session.Model
					m.modelName = msg.Session.Model
				}
				m.inputTokens = msg.Session.InputTokens
				m.outputTokens = msg.Session.OutputTokens
				m.costUSD = msg.Session.CostUSD
				m.numTurns = msg.Session.NumTurns
				if msg.Session.Status != "" {
					s.Status = msg.Session.Status
				} else if s.Status == "" {
					s.Status = "running"
				}
				if msg.Session.Action != "" {
					s.Action = msg.Session.Action
				} else if s.Action == "" {
					s.Action = "monitoring"
				}
				s.StartedAt = msg.Session.StartedAt
				s.EndedAt = msg.Session.EndedAt
				s.LastUpdate = time.Now()
				if !msg.Session.StartedAt.IsZero() {
					m.startTime = msg.Session.StartedAt
					m.elapsed = time.Since(msg.Session.StartedAt)
				}
			}
		}
		m.applySnapshotSpawnStatus(msg.Spawns)
		return m, waitForEvent(m.eventCh)

	case SessionLiveMsg:
		m.live = true
		return m, waitForEvent(m.eventCh)

	case AgentEventMsg:
		m.handleEvent(msg.Event)
		return m, waitForEvent(m.eventCh)

	case AgentRawOutputMsg:
		scope := m.sessionScope(msg.SessionID)
		if scope == "" && m.sessionID > 0 {
			scope = m.sessionScope(m.sessionID)
		}
		m.handleRawChunk(scope, msg.Data)
		return m, waitForEvent(m.eventCh)

	case SpawnStatusMsg:
		m.applyLiveSpawnStatus(msg.Spawns)
		return m, waitForEvent(m.eventCh)

	case GuardrailViolationMsg:
		m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Bold(true).Render(
			fmt.Sprintf("[guardrail] %s attempted %s — turn interrupted", msg.Role, msg.Tool)))
		m.addSimplifiedLine("", dimStyle.Render(fmt.Sprintf("guardrail blocked %s for %s", msg.Tool, msg.Role)))
		return m, waitForEvent(m.eventCh)

	case AgentStartedMsg:
		sessionID := msg.SessionID
		if sessionID <= 0 {
			sessionID = m.sessionID
		}
		if sessionID <= 0 {
			return m, waitForEvent(m.eventCh)
		}
		m.finalizeWaitingSessions(sessionID)
		m.sessionID = sessionID
		now := time.Now()
		s := m.ensureSession(sessionID)
		if s == nil {
			return m, waitForEvent(m.eventCh)
		}
		resuming := isWaitingSessionStatus(s.Status)
		if s.Agent == "" {
			s.Agent = m.agentName
		}
		if s.Profile == "" {
			s.Profile = m.loopStepProfile
		}
		s.Status = "running"
		if resuming {
			s.Action = "resumed"
		} else {
			s.Action = "starting"
		}
		if !resuming || s.StartedAt.IsZero() {
			s.StartedAt = now
		}
		s.EndedAt = time.Time{}
		s.LastUpdate = now
		scope := m.sessionScope(sessionID)
		if !resuming {
			delete(m.lastMessageByScope, scope)
			delete(m.assistantMessagesByScope, scope)
		}
		delete(m.finalizedTurnHexByScope, scope)
		turnLabel := fmt.Sprintf(">>> Turn #%d", sessionID)
		if msg.TurnHexID != "" {
			turnLabel += fmt.Sprintf(" [%s]", msg.TurnHexID)
		}
		if resuming {
			turnLabel += " resumed"
		} else {
			turnLabel += " started"
		}
		m.addScopedLine(scope, dimStyle.Render(turnLabel))
		m.addSimplifiedLine(scope, dimStyle.Render("turn started"))
		return m, waitForEvent(m.eventCh)

	case AgentPromptMsg:
		scope := m.sessionScope(msg.SessionID)
		if scope == "" && m.sessionID > 0 {
			scope = m.sessionScope(m.sessionID)
		}
		m.recordAgentPrompt(scope, msg)
		return m, waitForEvent(m.eventCh)

	case AgentFinishedMsg:
		scope := m.sessionScope(msg.SessionID)
		if scope == "" && m.sessionID > 0 {
			scope = m.sessionScope(m.sessionID)
		}
		m.flushRawRemainder(scope)
		s := m.ensureSession(msg.SessionID)
		now := time.Now()
		if s != nil {
			s.LastUpdate = now
		}
		logEntity := "Turn"
		logID := msg.SessionID
		if msg.SessionID < 0 {
			logEntity = "Spawn"
			logID = -msg.SessionID
		}
		hexTag := ""
		if msg.TurnHexID != "" {
			hexTag = fmt.Sprintf(" [%s]", msg.TurnHexID)
		}
		if msg.Result != nil {
			if s != nil {
				s.EndedAt = now
				switch {
				case msg.Err != nil:
					s.Status = "failed"
					s.Action = "error"
				case msg.WaitForSpawns:
					s.Status = "waiting_for_spawns"
					s.Action = "waiting for spawns"
				case msg.Result.ExitCode == 0:
					s.Status = "completed"
				default:
					s.Status = "failed"
				}
				if msg.Err == nil && !msg.WaitForSpawns {
					s.Action = fmt.Sprintf("finished (exit=%d)", msg.Result.ExitCode)
				}
			}
			if msg.Err != nil {
				m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
					fmt.Sprintf("<<< %s #%d%s error: %v (exit=%d, %s)",
						logEntity, logID, hexTag, msg.Err, msg.Result.ExitCode, msg.Result.Duration.Round(time.Second))))
				m.addSimplifiedLine(scope, dimStyle.Render("turn failed"))
			} else if msg.WaitForSpawns {
				m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("<<< %s #%d%s waiting for spawns (exit=%d, %s)",
					logEntity, logID, hexTag, msg.Result.ExitCode, msg.Result.Duration.Round(time.Second))))
				m.addSimplifiedLine(scope, dimStyle.Render("waiting for spawns"))
			} else {
				m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("<<< %s #%d%s finished (exit=%d, %s)",
					logEntity, logID, hexTag, msg.Result.ExitCode, msg.Result.Duration.Round(time.Second))))
				m.addSimplifiedLine(scope, dimStyle.Render("turn finished"))
			}
		} else if msg.Err != nil {
			if s != nil {
				s.EndedAt = now
				s.Status = "failed"
				s.Action = "error"
			}
			m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("<<< %s #%d%s error: %v", logEntity, logID, hexTag, msg.Err)))
			m.addSimplifiedLine(scope, dimStyle.Render("turn failed"))
		}
		if msg.WaitForSpawns {
			delete(m.finalizedTurnHexByScope, scope)
		} else {
			m.finalizedTurnHexByScope[scope] = msg.TurnHexID
			m.updateFinalMessageSnapshot(scope, msg.TurnHexID)
		}
		return m, waitForEvent(m.eventCh)

	case AgentLoopDoneMsg:
		m.flushAllRawRemainders()
		m.done = true
		m.exitErr = msg.Err
		if msg.Err != nil {
			m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("Loop error: %v", msg.Err)))
		} else {
			m.addLine("")
			m.addLine(resultLabelStyle.Render("Agent loop finished."))
		}
		// Show cost/token summary.
		if m.costUSD > 0 || m.inputTokens > 0 {
			m.addLine(dimStyle.Render(fmt.Sprintf("  Total: $%.4f, %d in / %d out tokens, %d turns",
				m.costUSD, m.inputTokens, m.outputTokens, m.numTurns)))
		}
		return m, nil

	case LoopStepStartMsg:
		m.loopCycle = msg.Cycle
		m.loopStep = msg.StepIndex
		m.loopStepProfile = msg.Profile
		m.loopRunHexID = msg.RunHexID
		m.loopStepHexID = msg.StepHexID
		if msg.TotalSteps > 0 {
			m.loopTotalSteps = msg.TotalSteps
		}
		m.addLine("")
		stepLine := fmt.Sprintf("[loop] Cycle %d, Step %d/%d: %s (x%d)",
			msg.Cycle+1, msg.StepIndex+1, m.loopTotalSteps, msg.Profile, msg.Turns)
		if msg.RunHexID != "" || msg.StepHexID != "" {
			stepLine += " ["
			if msg.RunHexID != "" {
				stepLine += "run:" + msg.RunHexID
			}
			if msg.StepHexID != "" {
				if msg.RunHexID != "" {
					stepLine += " "
				}
				stepLine += "step:" + msg.StepHexID
			}
			stepLine += "]"
		}
		m.addLine(initLabelStyle.Render(stepLine))
		m.addSimplifiedLine("", dimStyle.Render("loop step started"))
		return m, waitForEvent(m.eventCh)

	case LoopStepEndMsg:
		if msg.TotalSteps > 0 {
			m.loopTotalSteps = msg.TotalSteps
		}
		stepEndLine := fmt.Sprintf("[loop] Step %d/%d: %s completed",
			msg.StepIndex+1, m.loopTotalSteps, msg.Profile)
		if msg.StepHexID != "" {
			stepEndLine += fmt.Sprintf(" [step:%s]", msg.StepHexID)
		}
		m.addLine(dimStyle.Render(stepEndLine))
		m.addSimplifiedLine("", dimStyle.Render("loop step completed"))
		return m, waitForEvent(m.eventCh)

	case LoopDoneMsg:
		m.flushAllRawRemainders()
		m.done = true
		m.exitErr = msg.Err
		if msg.Err != nil && msg.Reason != "cancelled" {
			m.addLine(lipgloss.NewStyle().Foreground(theme.ColorRed).Render(
				fmt.Sprintf("Loop error: %v", msg.Err)))
		} else {
			m.addLine("")
			m.addLine(resultLabelStyle.Render(fmt.Sprintf("Loop finished (%s).", msg.Reason)))
		}
		// Show cost/token summary.
		if m.costUSD > 0 || m.inputTokens > 0 {
			m.addLine(dimStyle.Render(fmt.Sprintf("  Total: $%.4f, %d in / %d out tokens, %d turns",
				m.costUSD, m.inputTokens, m.outputTokens, m.numTurns)))
		}
		return m, nil

	case tickMsg:
		m.elapsed = time.Since(m.startTime)
		if m.projectStore != nil && (m.lastDataLoad.IsZero() || time.Since(m.lastDataLoad) >= 2*time.Second) {
			m.reloadProjectData()
		}
		// keep scroll clamped as filtered line counts change with selection/focus.
		ms := m.maxScroll()
		if m.scrollPos > ms {
			m.scrollPos = ms
		}
		return m, tickEvery()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case DetachMsg:
		return m, tea.Quit
	}

	return m, nil
}

// Done returns whether the agent loop has finished.
func (m Model) Done() bool {
	return m.done
}

func (m *Model) applyLiveSpawnStatus(spawns []SpawnInfo) {
	changes := m.updateSpawnStatus(spawns)
	for _, sp := range changes {
		scope := m.spawnScope(sp.ID)
		m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("[spawn #%d] %s -> %s", sp.ID, sp.Profile, sp.Status)))
		m.addSimplifiedLine(scope, dimStyle.Render("spawn update"))
		m.bumpStats(scope, func(st *detailStats) { st.Spawns++ })
		if sp.Question != "" {
			m.addScopedLine(scope, dimStyle.Render("  Q: "+truncate(sp.Question, 180)))
		}
	}
}

func (m *Model) applySnapshotSpawnStatus(spawns []SpawnInfo) {
	_ = m.updateSpawnStatus(spawns)
}

func (m *Model) updateSpawnStatus(spawns []SpawnInfo) []SpawnInfo {
	m.spawns = spawns
	now := time.Now()
	nextSeen := make(map[int]time.Time, len(spawns))
	nextStatus := make(map[int]string, len(spawns))
	activeByParent := make(map[int]bool)
	seenByParent := make(map[int]bool)
	changes := make([]SpawnInfo, 0, len(spawns))

	for _, sp := range spawns {
		firstSeen, ok := m.spawnFirstSeen[sp.ID]
		if !ok {
			firstSeen = now
		}
		nextSeen[sp.ID] = firstSeen
		nextStatus[sp.ID] = sp.Status
		if sp.ParentTurnID > 0 {
			seenByParent[sp.ParentTurnID] = true
			if !isTerminalSpawnStatus(sp.Status) {
				activeByParent[sp.ParentTurnID] = true
			}
		}
		prev, hadPrev := m.spawnStatus[sp.ID]
		if !hadPrev || prev != sp.Status {
			changes = append(changes, sp)
		}
	}

	m.spawnFirstSeen = nextSeen
	m.spawnStatus = nextStatus
	for _, sid := range m.sessionOrder {
		s := m.sessions[sid]
		if s == nil || !isWaitingSessionStatus(s.Status) {
			continue
		}
		if !seenByParent[sid] || activeByParent[sid] {
			continue
		}
		s.Status = "completed"
		if s.Action == "" || strings.Contains(strings.ToLower(s.Action), "wait") {
			s.Action = "wait complete"
		}
		s.LastUpdate = now
	}
	return changes
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	moveSelection := func(delta int) {
		switch m.leftSection {
		case leftSectionIssues:
			if len(m.issues) == 0 {
				m.selectedIssue = 0
				return
			}
			m.selectedIssue += delta
			if m.selectedIssue < 0 {
				m.selectedIssue = 0
			}
			if m.selectedIssue >= len(m.issues) {
				m.selectedIssue = len(m.issues) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		case leftSectionDocs:
			if len(m.docs) == 0 {
				m.selectedDoc = 0
				return
			}
			m.selectedDoc += delta
			if m.selectedDoc < 0 {
				m.selectedDoc = 0
			}
			if m.selectedDoc >= len(m.docs) {
				m.selectedDoc = len(m.docs) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		case leftSectionPlan:
			if m.plan == nil || len(m.plan.Phases) == 0 {
				m.selectedPhase = 0
				return
			}
			m.selectedPhase += delta
			if m.selectedPhase < 0 {
				m.selectedPhase = 0
			}
			if m.selectedPhase >= len(m.plan.Phases) {
				m.selectedPhase = len(m.plan.Phases) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		case leftSectionLogs:
			if len(m.turns) == 0 {
				m.selectedTurn = 0
				return
			}
			m.selectedTurn += delta
			if m.selectedTurn < 0 {
				m.selectedTurn = 0
			}
			if m.selectedTurn >= len(m.turns) {
				m.selectedTurn = len(m.turns) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		default:
			entries := m.commandEntries()
			if len(entries) == 0 {
				m.selectedEntry = 0
				return
			}
			m.selectedEntry += delta
			if m.selectedEntry < 0 {
				m.selectedEntry = 0
			}
			if m.selectedEntry >= len(entries) {
				m.selectedEntry = len(entries) - 1
			}
			if m.detailLayer == detailLayerRaw {
				m.scrollToBottom()
				m.autoScroll = true
			} else {
				m.scrollPos = 0
				m.autoScroll = false
			}
		}
	}
	cycleActiveAgent := func(delta int) {
		m.setLeftSection(leftSectionAgents)
		entries := m.commandEntries()
		if len(entries) <= 1 {
			return
		}

		active := make([]int, 0, len(entries)-1)
		for i := 1; i < len(entries); i++ {
			if entries[i].scope == "" {
				continue
			}
			if isActiveSessionStatus(entries[i].status) {
				active = append(active, i)
			}
		}
		if len(active) == 0 {
			for i := 1; i < len(entries); i++ {
				if entries[i].scope != "" {
					active = append(active, i)
				}
			}
		}
		if len(active) == 0 {
			return
		}

		pos := -1
		for i, idx := range active {
			if idx == m.selectedEntry {
				pos = i
				break
			}
		}
		if pos == -1 {
			if delta >= 0 {
				m.selectedEntry = active[0]
			} else {
				m.selectedEntry = active[len(active)-1]
			}
			if m.detailLayer == detailLayerRaw {
				m.scrollToBottom()
				m.autoScroll = true
			} else {
				m.scrollPos = 0
				m.autoScroll = false
			}
			return
		}

		if delta >= 0 {
			pos = (pos + 1) % len(active)
		} else {
			pos = (pos - 1 + len(active)) % len(active)
		}
		m.selectedEntry = active[pos]
		if m.detailLayer == detailLayerRaw {
			m.scrollToBottom()
			m.autoScroll = true
		} else {
			m.scrollPos = 0
			m.autoScroll = false
		}
	}
	moveToBoundary := func(start bool) {
		switch m.leftSection {
		case leftSectionIssues:
			if len(m.issues) == 0 {
				m.selectedIssue = 0
			} else if start {
				m.selectedIssue = 0
			} else {
				m.selectedIssue = len(m.issues) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		case leftSectionDocs:
			if len(m.docs) == 0 {
				m.selectedDoc = 0
			} else if start {
				m.selectedDoc = 0
			} else {
				m.selectedDoc = len(m.docs) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		case leftSectionPlan:
			if m.plan == nil || len(m.plan.Phases) == 0 {
				m.selectedPhase = 0
			} else if start {
				m.selectedPhase = 0
			} else {
				m.selectedPhase = len(m.plan.Phases) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		case leftSectionLogs:
			if len(m.turns) == 0 {
				m.selectedTurn = 0
			} else if start {
				m.selectedTurn = 0
			} else {
				m.selectedTurn = len(m.turns) - 1
			}
			m.scrollPos = 0
			m.autoScroll = false
		default:
			entries := m.commandEntries()
			if len(entries) == 0 {
				m.selectedEntry = 0
			} else if start {
				m.selectedEntry = 0
			} else {
				m.selectedEntry = len(entries) - 1
			}
			if m.detailLayer == detailLayerRaw {
				m.scrollToBottom()
				m.autoScroll = true
			} else {
				m.scrollPos = 0
				m.autoScroll = false
			}
		}
	}

	switch msg.String() {
	case "q":
		if m.done {
			return m, tea.Quit
		}
		return m, nil
	case "esc", "backspace":
		if m.done {
			return m, func() tea.Msg { return BackToSelectorMsg{} }
		}
		return m, nil
	case "ctrl+d":
		if m.sessionModeID > 0 && !m.done {
			// Detach from the session without stopping the agent.
			return m, func() tea.Msg {
				return DetachMsg{SessionID: m.sessionModeID}
			}
		}
		// Not in session mode: page down.
		m.scrollDown(m.detailViewportHeight() / 2)
	case "tab":
		if m.focus == focusDetail {
			m.focus = focusCommand
		} else {
			m.focus = focusDetail
		}
	case "left", "h":
		m.focus = focusCommand
	case "right", "l":
		m.focus = focusDetail
	case "1":
		m.setLeftSection(leftSectionAgents)
	case "2":
		m.setLeftSection(leftSectionIssues)
	case "3":
		m.setLeftSection(leftSectionDocs)
	case "4":
		m.setLeftSection(leftSectionPlan)
	case "5":
		m.setLeftSection(leftSectionLogs)
	case "t":
		if m.focus == focusDetail && m.leftSection == leftSectionAgents {
			m.cycleDetailLayer(1)
		}
	case "T":
		if m.focus == focusDetail && m.leftSection == leftSectionAgents {
			m.cycleDetailLayer(-1)
		}
	case "]", "n":
		cycleActiveAgent(1)
	case "[", "p":
		cycleActiveAgent(-1)
	case "ctrl+c":
		if m.done {
			return m, tea.Quit
		}
		if m.stopping {
			// Second Ctrl+C: force quit.
			return m, tea.Quit
		}
		// First Ctrl+C: cancel the agent.
		m.stopping = true
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		m.addLine("")
		m.addLine(lipgloss.NewStyle().Foreground(theme.ColorYellow).Bold(true).Render(
			"Stopping agent... (press Ctrl+C again to force quit)"))
		return m, nil
	case "j", "down":
		if m.focus == focusCommand {
			moveSelection(1)
		} else {
			m.scrollDown(1)
		}
	case "k", "up":
		if m.focus == focusCommand {
			moveSelection(-1)
		} else {
			m.scrollUp(1)
		}
	case "pgdown":
		if m.focus == focusDetail {
			m.scrollDown(m.detailViewportHeight() / 2)
		}
	case "pgup", "ctrl+u":
		if m.focus == focusDetail {
			m.scrollUp(m.detailViewportHeight() / 2)
		}
	case "home", "g":
		if m.focus == focusDetail {
			m.scrollPos = 0
			m.autoScroll = false
		} else {
			moveToBoundary(true)
		}
	case "end", "G":
		if m.focus == focusDetail {
			m.scrollToBottom()
			m.autoScroll = true
		} else {
			moveToBoundary(false)
		}
	}
	return m, nil
}

func (m *Model) setLeftSection(section leftPanelSection) {
	if m.leftSection == section {
		return
	}
	m.leftSection = section
	switch section {
	case leftSectionAgents:
		if m.detailLayer == detailLayerRaw {
			m.scrollToBottom()
			m.autoScroll = true
		} else {
			m.scrollPos = 0
			m.autoScroll = false
		}
	default:
		m.scrollPos = 0
		m.autoScroll = false
	}
}

// --- Scrolling ---

// totalLines returns the count of visible lines including a partial streaming line.
func (m Model) totalLines() int {
	return len(m.detailLines(m.rcWidth()))
}

func (m *Model) scrollDown(n int) {
	ms := m.maxScroll()
	m.scrollPos += n
	if m.scrollPos > ms {
		m.scrollPos = ms
	}
	m.autoScroll = m.scrollPos >= ms
}

func (m *Model) scrollUp(n int) {
	m.scrollPos -= n
	if m.scrollPos < 0 {
		m.scrollPos = 0
	}
	m.autoScroll = false
}

func (m *Model) scrollToBottom() {
	m.scrollPos = m.maxScroll()
}

func (m Model) maxScroll() int {
	vh := m.detailViewportHeight()
	total := m.totalLines()
	if total <= vh {
		return 0
	}
	return total - vh
}

func (m Model) selectedScope() string {
	if m.leftSection != leftSectionAgents {
		return ""
	}
	entries := m.commandEntries()
	if len(entries) == 0 {
		return ""
	}
	idx := m.selectedEntry
	if idx < 0 {
		idx = 0
	}
	if idx >= len(entries) {
		idx = len(entries) - 1
	}
	return entries[idx].scope
}

func (m Model) scopeVisible(scope string) bool {
	selected := m.selectedScope()
	if selected == "" {
		return true
	}
	// Global lines remain visible in every filtered detail view.
	return scope == "" || scope == selected
}

func (m Model) shouldPrefixAllOutput() bool {
	if m.selectedScope() != "" {
		return false
	}
	scopes := make(map[string]struct{}, 4)
	for _, line := range m.lines {
		if line.scope == "" {
			continue
		}
		scopes[line.scope] = struct{}{}
		if len(scopes) > 1 {
			return true
		}
	}
	if m.currentScope != "" {
		scopes[m.currentScope] = struct{}{}
	}
	return len(scopes) > 1
}

func (m Model) scopePrefix(scope string) string {
	if strings.HasPrefix(scope, "session:") {
		if sid := m.sessionIDForScope(scope); sid > 0 {
			if s, ok := m.sessions[sid]; ok && s != nil {
				if s.Profile != "" {
					return fmt.Sprintf("[turn #%d:%s]", sid, s.Profile)
				}
			}
			return fmt.Sprintf("[turn #%d]", sid)
		}
	}
	if strings.HasPrefix(scope, "spawn:") {
		id := strings.TrimPrefix(scope, "spawn:")
		if id != "" {
			return "[spawn#" + id + "]"
		}
	}
	return "[scope]"
}

func (m Model) maybePrefixedLine(scope, text string, enable bool) string {
	if !enable || scope == "" {
		return text
	}
	return dimStyle.Render(m.scopePrefix(scope)+" ") + text
}

func (m Model) filteredLines() []string {
	if len(m.lines) == 0 {
		return nil
	}
	selected := m.selectedScope()
	prefixAll := m.shouldPrefixAllOutput()
	if selected == "" {
		out := make([]string, 0, len(m.lines))
		for _, line := range m.lines {
			out = append(out, m.maybePrefixedLine(line.scope, line.text, prefixAll))
		}
		return out
	}
	out := make([]string, 0, len(m.lines))
	for _, line := range m.lines {
		if line.scope == "" || line.scope == selected {
			out = append(out, m.maybePrefixedLine(line.scope, line.text, false))
		}
	}
	return out
}

// rcHeight returns the right panel content height (lines of text visible).
func (m Model) rcHeight() int {
	_, vf := rightPanelStyle.GetFrameSize()
	ph := m.height - 2 // header + status bar
	h := ph - vf
	if h < 1 {
		return 1
	}
	return h
}

// rcWidth returns the right panel content width (chars per line).
func (m Model) rcWidth() int {
	hf, _ := rightPanelStyle.GetFrameSize()
	rw := m.width - leftPanelOuterWidth
	if rw < 1 {
		rw = m.width
	}
	w := rw - hf
	if w < 1 {
		return 1
	}
	return w
}

// --- Content management ---

func (m Model) sessionScope(sessionID int) string {
	if sessionID == 0 {
		return ""
	}
	if sessionID < 0 {
		return m.spawnScope(-sessionID)
	}
	return fmt.Sprintf("session:%d", sessionID)
}

func (m Model) spawnScope(spawnID int) string {
	if spawnID <= 0 {
		return ""
	}
	return fmt.Sprintf("spawn:%d", spawnID)
}

func (m Model) sessionIDForScope(scope string) int {
	if !strings.HasPrefix(scope, "session:") {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimPrefix(scope, "session:"))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func splitRenderableLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

// addLine adds a completed styled line in the global scope.
func (m *Model) addLine(line string) {
	m.addScopedLine("", line)
}

// addScopedLine adds a completed styled line under a specific scope.
func (m *Model) addScopedLine(scope, line string) {
	for _, part := range splitRenderableLines(line) {
		m.lines = append(m.lines, scopedLine{
			scope: scope,
			text:  part,
		})
	}
	if m.autoScroll && m.scopeVisible(scope) {
		m.scrollToBottom()
	}
}

func (m *Model) ensureStreamBuf() *strings.Builder {
	if m.streamBuf == nil {
		m.streamBuf = &strings.Builder{}
	}
	return m.streamBuf
}

func (m *Model) ensureToolInputBuf() *strings.Builder {
	if m.toolInputBuf == nil {
		m.toolInputBuf = &strings.Builder{}
	}
	return m.toolInputBuf
}

func (m *Model) switchStreamScope(scope string) {
	if m.currentScope == scope {
		return
	}
	m.flushStream()
	m.currentScope = scope
	m.currentBlockType = ""
	m.currentToolName = ""
	m.ensureToolInputBuf().Reset()
}

func (m *Model) ensureSession(sessionID int) *sessionStatus {
	if sessionID <= 0 {
		return nil
	}
	if s, ok := m.sessions[sessionID]; ok {
		return s
	}
	now := time.Now()
	s := &sessionStatus{
		ID:         sessionID,
		Agent:      m.agentName,
		Profile:    m.loopStepProfile,
		Status:     "running",
		Action:     "starting",
		StartedAt:  now,
		LastUpdate: now,
	}
	m.sessions[sessionID] = s
	m.sessionOrder = append(m.sessionOrder, sessionID)
	return s
}

func (m *Model) setSessionAction(sessionID int, action string) {
	if sessionID <= 0 {
		return
	}
	s := m.ensureSession(sessionID)
	if s == nil {
		return
	}
	if action != "" {
		s.Action = action
	}
	s.LastUpdate = time.Now()
}

func isWaitingSessionStatus(status string) bool {
	switch status {
	case "waiting_for_spawns", "waiting":
		return true
	default:
		return false
	}
}

func isActiveSessionStatus(status string) bool {
	switch status {
	case "running", "awaiting_input":
		return true
	default:
		return isWaitingSessionStatus(status)
	}
}

func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
		return true
	default:
		return false
	}
}

func (m *Model) finalizeWaitingSessions(exceptSessionID int) {
	now := time.Now()
	for _, sid := range m.sessionOrder {
		if sid == exceptSessionID {
			continue
		}
		s := m.sessions[sid]
		if s == nil || !isWaitingSessionStatus(s.Status) {
			continue
		}
		s.Status = "completed"
		if s.Action == "" || strings.Contains(strings.ToLower(s.Action), "wait") {
			s.Action = "wait complete"
		}
		if s.EndedAt.IsZero() {
			s.EndedAt = now
		}
		s.LastUpdate = now
	}
}

// flushStream flushes the streaming buffer to a completed line.
func (m *Model) flushStream() {
	if m.streamBuf == nil || m.streamBuf.Len() == 0 {
		return
	}
	styled := m.styleDelta(m.streamBuf.String())
	m.addScopedLine(m.currentScope, styled)
	m.streamBuf.Reset()
}

// appendDelta processes streaming delta text and flushes completed lines.
func (m *Model) appendDelta(text string) {
	buf := m.ensureStreamBuf()
	for _, r := range text {
		if r == '\n' {
			m.flushStream()
		} else {
			buf.WriteRune(r)
		}
	}
	if m.autoScroll && m.scopeVisible(m.currentScope) {
		m.scrollToBottom()
	}
}

// styleDelta applies the appropriate style based on current block type.
func (m Model) styleDelta(text string) string {
	switch m.currentBlockType {
	case "thinking":
		return thinkingTextStyle.Render(text)
	case "tool_use":
		return toolInputStyle.Render(text)
	default:
		return textStyle.Render(text)
	}
}

// renderToolInput parses the accumulated tool JSON and shows key fields.
func (m *Model) renderToolInput(scope, toolName, rawJSON string) {
	var data map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		// Fallback: show truncated raw JSON.
		if len(rawJSON) > 200 {
			rawJSON = rawJSON[:200] + "..."
		}
		m.addScopedLine(scope, toolInputStyle.Render(rawJSON))
		return
	}

	// Extract the most useful fields based on tool name.
	var parts []string
	switch toolName {
	case "Bash":
		if cmd, ok := data["command"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(cmd, 200)))
		}
		if desc, ok := data["description"].(string); ok {
			parts = append(parts, dimStyle.Render("# "+truncate(desc, 100)))
		}
	case "Read":
		if fp, ok := data["file_path"].(string); ok {
			parts = append(parts, toolInputStyle.Render(fp))
		}
	case "Write":
		if fp, ok := data["file_path"].(string); ok {
			parts = append(parts, toolInputStyle.Render(fp))
		}
	case "Edit":
		if fp, ok := data["file_path"].(string); ok {
			parts = append(parts, toolInputStyle.Render(fp))
		}
	case "Grep":
		if pat, ok := data["pattern"].(string); ok {
			parts = append(parts, toolInputStyle.Render("pattern="+truncate(pat, 100)))
		}
		if p, ok := data["path"].(string); ok {
			parts = append(parts, dimStyle.Render("path="+p))
		}
	case "Glob":
		if pat, ok := data["pattern"].(string); ok {
			parts = append(parts, toolInputStyle.Render("pattern="+truncate(pat, 100)))
		}
	case "Task":
		if desc, ok := data["description"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(desc, 200)))
		}
	case "WebFetch":
		if url, ok := data["url"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(url, 200)))
		}
	case "WebSearch":
		if q, ok := data["query"].(string); ok {
			parts = append(parts, toolInputStyle.Render(truncate(q, 200)))
		}
	}

	if len(parts) == 0 {
		// Generic fallback: show all string-valued keys.
		for k, v := range data {
			if s, ok := v.(string); ok {
				parts = append(parts, dimStyle.Render(k+"=")+toolInputStyle.Render(truncate(s, 100)))
				if len(parts) >= 3 {
					break
				}
			}
		}
	}

	for _, p := range parts {
		m.addScopedLine(scope, "  "+p)
	}
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		if max <= 3 {
			return s[:max]
		}
		return s[:max-3] + "..."
	}
	return s
}

func (m *Model) handleRawChunk(scope, chunk string) {
	if chunk == "" {
		return
	}
	if scope == "" {
		scope = m.sessionScope(m.sessionID)
	}
	if scope == "" {
		scope = ""
	}

	// Raw output is line-oriented and independent from Claude block deltas.
	m.switchStreamScope(scope)
	data := m.rawRemainder[scope] + strings.ReplaceAll(chunk, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	lines := strings.Split(data, "\n")
	if len(lines) == 0 {
		return
	}
	m.rawRemainder[scope] = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		m.handleRawLine(scope, line)
	}
}

func (m *Model) flushRawRemainder(scope string) {
	if rem, ok := m.rawRemainder[scope]; ok && strings.TrimSpace(rem) != "" {
		m.handleRawLine(scope, rem)
	}
	delete(m.rawRemainder, scope)
	m.flushStderrRepeat(scope)
}

func (m *Model) flushAllRawRemainders() {
	for scope := range m.rawRemainder {
		m.flushRawRemainder(scope)
	}
}

func (m *Model) handleRawLine(scope, line string) {
	if strings.TrimSpace(line) == "" {
		m.addScopedLine(scope, "")
		return
	}

	if m.renderVibeStreamingLine(scope, line) {
		return
	}

	if m.renderStreamEventLine(scope, line) {
		return
	}

	if m.renderStderrLine(scope, line) {
		return
	}

	trimmed := truncate(line, 400)
	m.addScopedLine(scope, textStyle.Render(trimmed))
	if sid := m.sessionIDForScope(scope); sid > 0 {
		m.setSessionAction(sid, "processing output")
	}
}

func (m *Model) renderVibeStreamingLine(scope, line string) bool {
	var msg struct {
		Role             string `json:"role"`
		Content          string `json:"content"`
		ReasoningContent string `json:"reasoning_content"`
		ToolCalls        []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return false
	}
	if msg.Role == "" {
		return false
	}

	switch msg.Role {
	case "system":
		m.addScopedLine(scope, initLabelStyle.Render("[vibe:system] context loaded"))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "loading context")
		}
	case "user":
		text := compactWhitespace(msg.Content)
		if strings.TrimSpace(text) == "" {
			text = "prompt received"
		}
		m.addScopedLine(scope, dimStyle.Render("[vibe:user] "+truncate(text, 200)))
		m.addSimplifiedLine(scope, dimStyle.Render("prompt received"))
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "processing prompt")
		}
	case "assistant":
		if msg.ReasoningContent != "" {
			text := compactWhitespace(msg.ReasoningContent)
			m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
			m.addScopedLine(scope, "  "+thinkingTextStyle.Render(truncate(text, 300)))
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "thinking")
			}
		}
		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			if name == "" {
				name = "tool"
			}
			m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", name)))
			m.recordToolCall(scope, name)
			if tc.Function.Arguments != "" {
				m.addScopedLine(scope, "  "+toolInputStyle.Render(truncate(compactWhitespace(tc.Function.Arguments), 200)))
			}
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "running "+name)
			}
		}
		if strings.TrimSpace(msg.Content) != "" {
			m.addScopedLine(scope, textLabelStyle.Render("[text]"))
			m.addScopedLine(scope, "  "+textStyle.Render(truncate(msg.Content, 500)))
			m.recordAssistantText(scope, msg.Content)
			if sid := m.sessionIDForScope(scope); sid > 0 {
				m.setSessionAction(sid, "responding")
			}
		}
	case "tool":
		m.markAssistantBoundary(scope)
		m.addScopedLine(scope, toolResultStyle.Render("[result]"))
		m.addSimplifiedLine(scope, dimStyle.Render("tool result"))
		if strings.TrimSpace(msg.Content) != "" {
			for _, part := range splitRenderableLines(truncate(msg.Content, 400)) {
				m.addScopedLine(scope, dimStyle.Render("  "+part))
			}
		}
		if sid := m.sessionIDForScope(scope); sid > 0 {
			m.setSessionAction(sid, "processing tool result")
		}
	default:
		return false
	}

	return true
}

// stderrCategory classifies a stderr line body.
type stderrCategory int

const (
	stderrGeneric     stderrCategory = iota
	stderrDeprecation                // Node.js deprecation warnings
	stderrInfo                       // Informational messages (YOLO mode, credentials, etc.)
)

// classifyStderr returns the category for a stderr line body (without the [stderr] prefix).
func classifyStderr(body string) stderrCategory {
	lower := strings.ToLower(body)
	// Deprecation warnings from Node.js.
	if strings.Contains(body, "DeprecationWarning:") ||
		strings.HasPrefix(body, "(Use `node --trace-deprecation") ||
		strings.HasPrefix(body, "(node:") {
		return stderrDeprecation
	}
	// Known informational messages.
	if strings.Contains(lower, "yolo mode") ||
		strings.Contains(lower, "loaded cached credentials") ||
		strings.Contains(lower, "hook registry initialized") ||
		strings.Contains(lower, "mode is enabled") {
		return stderrInfo
	}
	return stderrGeneric
}

// renderStderrLine handles lines that start with "[stderr] ". Returns true if
// the line was consumed (callers should skip further processing).
func (m *Model) renderStderrLine(scope, line string) bool {
	if !strings.HasPrefix(line, "[stderr] ") {
		return false
	}
	body := strings.TrimSpace(strings.TrimPrefix(line, "[stderr] "))
	if body == "" {
		// Empty stderr line — consume silently.
		return true
	}

	// Deduplication: suppress consecutive identical stderr within a scope.
	if last, ok := m.lastStderrByScope[scope]; ok && last == body {
		m.stderrRepeatCount[scope]++
		return true
	}
	// Flush any pending repeat annotation before rendering a new line.
	m.flushStderrRepeat(scope)
	m.lastStderrByScope[scope] = body
	m.stderrRepeatCount[scope] = 0

	truncBody := truncate(body, 400)
	switch classifyStderr(body) {
	case stderrDeprecation:
		m.addScopedLine(scope, stderrDimStyle.Render("[stderr] "+truncBody))
	case stderrInfo:
		m.addScopedLine(scope, dimStyle.Render("[stderr] "+truncBody))
	default: // stderrGeneric
		m.addScopedLine(scope, stderrLabelStyle.Render("[stderr]")+" "+dimStyle.Render(truncBody))
	}
	return true
}

// flushStderrRepeat emits a suppression annotation if consecutive identical
// stderr lines were deduplicated for the given scope, then resets state.
func (m *Model) flushStderrRepeat(scope string) {
	if count := m.stderrRepeatCount[scope]; count > 0 {
		note := fmt.Sprintf("  (%d identical lines suppressed)", count)
		m.addScopedLine(scope, stderrDimStyle.Render(note))
	}
	delete(m.lastStderrByScope, scope)
	delete(m.stderrRepeatCount, scope)
}

// renderStreamEventLine attempts to parse a raw JSON line as a codex, gemini,
// or claude stream event and renders it in a human-readable form. Returns true
// if the line was handled.
func (m *Model) renderStreamEventLine(scope, line string) bool {
	// Quick check: must look like JSON with a "type" field.
	if len(line) == 0 || line[0] != '{' {
		return false
	}

	// Peek at the type field to decide which parser to use.
	var peek struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &peek); err != nil || peek.Type == "" {
		return false
	}

	// Try codex event types first (they use dotted names).
	if m.renderCodexStreamLine(scope, line, peek.Type) {
		return true
	}

	// Try gemini event types.
	if m.renderGeminiStreamLine(scope, line, peek.Type) {
		return true
	}

	// Try claude event types.
	if m.renderClaudeStreamLine(scope, line, peek.Type) {
		return true
	}

	return false
}

// renderCodexStreamLine handles codex JSON events from claude_stream recordings.
func (m *Model) renderCodexStreamLine(scope, line, eventType string) bool {
	switch eventType {
	case "thread.started":
		var ev struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		m.addScopedLine(scope, initLabelStyle.Render(fmt.Sprintf("[init] session=%s", ev.ThreadID)))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		return true

	case "turn.started", "item.started", "item.updated":
		// Skip intermediate events silently.
		return true

	case "item.completed":
		var ev struct {
			Item *struct {
				Type             string            `json:"type"`
				Text             string            `json:"text"`
				Command          string            `json:"command"`
				AggregatedOutput string            `json:"aggregated_output"`
				ExitCode         *int              `json:"exit_code"`
				Status           string            `json:"status"`
				Server           string            `json:"server"`
				Tool             string            `json:"tool"`
				Arguments        json.RawMessage   `json:"arguments"`
				Changes          []json.RawMessage `json:"changes"`
			} `json:"item"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Item == nil {
			return false
		}
		switch ev.Item.Type {
		case "agent_message":
			if strings.TrimSpace(ev.Item.Text) == "" {
				return true
			}
			m.addScopedLine(scope, textLabelStyle.Render("[text]"))
			m.addScopedLine(scope, "  "+textStyle.Render(truncate(ev.Item.Text, 500)))
			m.recordAssistantText(scope, ev.Item.Text)
		case "reasoning":
			if strings.TrimSpace(ev.Item.Text) == "" {
				return true
			}
			m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
			m.addScopedLine(scope, "  "+thinkingTextStyle.Render(truncate(compactWhitespace(ev.Item.Text), 300)))
		case "command_execution":
			cmd := ev.Item.Command
			if cmd == "" {
				cmd = "command"
			}
			m.addScopedLine(scope, toolLabelStyle.Render("[tool:Bash]"))
			m.recordToolCall(scope, "Bash")
			m.addScopedLine(scope, "  "+toolInputStyle.Render(truncate(cmd, 200)))
			if ev.Item.AggregatedOutput != "" {
				m.addScopedLine(scope, "  "+dimStyle.Render(truncate(ev.Item.AggregatedOutput, 200)))
			}
		case "mcp_tool_call":
			toolName := strings.Trim(strings.Join([]string{ev.Item.Server, ev.Item.Tool}, "."), ".")
			if toolName == "" {
				toolName = "mcp"
			}
			m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", toolName)))
			m.recordToolCall(scope, toolName)
			if len(ev.Item.Arguments) > 0 {
				m.addScopedLine(scope, "  "+toolInputStyle.Render(truncate(string(ev.Item.Arguments), 200)))
			}
		case "file_change":
			m.addScopedLine(scope, toolLabelStyle.Render("[file]"))
			m.addSimplifiedLine(scope, dimStyle.Render("file change"))
		default:
			return true
		}
		return true

	case "turn.completed":
		var ev struct {
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		json.Unmarshal([]byte(line), &ev)
		summary := "done"
		if ev.Usage != nil {
			summary = fmt.Sprintf("in=%d out=%d", ev.Usage.InputTokens, ev.Usage.OutputTokens)
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		return true

	case "turn.failed":
		var ev struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal([]byte(line), &ev)
		msg := "failed"
		if ev.Error != nil && ev.Error.Message != "" {
			msg = truncate(ev.Error.Message, 200)
		}
		m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error] "+msg))
		m.addSimplifiedLine(scope, dimStyle.Render("error"))
		return true

	case "error":
		// Codex top-level error event.
		var ev struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal([]byte(line), &ev)
		msg := "unknown error"
		if ev.Error != nil && ev.Error.Message != "" {
			msg = truncate(ev.Error.Message, 200)
		}
		m.addScopedLine(scope, lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error] "+msg))
		m.addSimplifiedLine(scope, dimStyle.Render("error"))
		return true

	default:
		return false
	}
}

// renderGeminiStreamLine handles gemini JSON events from claude_stream recordings.
func (m *Model) renderGeminiStreamLine(scope, line, eventType string) bool {
	switch eventType {
	case "init":
		var ev struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		m.addScopedLine(scope, initLabelStyle.Render(fmt.Sprintf("[init] model=%s", ev.Model)))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		return true

	case "message":
		var ev struct {
			Role    string `json:"role"`
			Content string `json:"content"`
			Delta   bool   `json:"delta"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		if ev.Role != "assistant" {
			// Skip user messages.
			return true
		}
		if strings.TrimSpace(ev.Content) == "" {
			return true
		}
		m.addScopedLine(scope, textLabelStyle.Render("[text]"))
		m.addScopedLine(scope, "  "+textStyle.Render(truncate(ev.Content, 500)))
		m.recordAssistantText(scope, ev.Content)
		return true

	case "tool_use":
		var ev struct {
			ToolName string `json:"tool_name"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		name := ev.ToolName
		if name == "" {
			name = "tool"
		}
		m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", name)))
		m.recordToolCall(scope, name)
		return true

	case "tool_result":
		m.markAssistantBoundary(scope)
		// Skip rendering tool results in detail raw text; they still define
		// the boundary for what counts as the final assistant message.
		return true

	case "result":
		var ev struct {
			Stats *struct {
				InputTokens  int     `json:"input_tokens"`
				OutputTokens int     `json:"output_tokens"`
				DurationMS   float64 `json:"duration_ms"`
			} `json:"stats"`
		}
		json.Unmarshal([]byte(line), &ev)
		summary := "done"
		if ev.Stats != nil {
			summary = fmt.Sprintf("in=%d out=%d", ev.Stats.InputTokens, ev.Stats.OutputTokens)
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		return true

	default:
		return false
	}
}

// renderClaudeStreamLine handles claude JSON events from claude_stream recordings.
func (m *Model) renderClaudeStreamLine(scope, line, eventType string) bool {
	switch eventType {
	case "system":
		var ev struct {
			Subtype string `json:"subtype"`
			Model   string `json:"model"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return false
		}
		if ev.Subtype == "init" {
			m.addScopedLine(scope, initLabelStyle.Render(fmt.Sprintf("[init] model=%s", ev.Model)))
			m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		}
		// Skip other system subtypes silently.
		return true

	case "assistant":
		var ev struct {
			Message *struct {
				Content []struct {
					Type  string          `json:"type"`
					Text  string          `json:"text"`
					Name  string          `json:"name"`
					Input json.RawMessage `json:"input"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Message == nil {
			return false
		}
		for _, block := range ev.Message.Content {
			switch block.Type {
			case "text":
				if strings.TrimSpace(block.Text) == "" {
					continue
				}
				text := block.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				m.addScopedLine(scope, textLabelStyle.Render("[text]"))
				m.addScopedLine(scope, "  "+textStyle.Render(text))
				m.recordAssistantText(scope, text)
			case "tool_use":
				name := block.Name
				if name == "" {
					name = "tool"
				}
				m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", name)))
				m.recordToolCall(scope, name)
				if len(block.Input) > 0 {
					m.renderToolInput(scope, name, string(block.Input))
				}
			case "thinking":
				if strings.TrimSpace(block.Text) == "" {
					continue
				}
				text := block.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
				m.addScopedLine(scope, "  "+thinkingTextStyle.Render(compactWhitespace(text)))
			}
		}
		return true

	case "user":
		// User events in Claude stream mode usually carry tool results; treat
		// them as the boundary for final-message capture.
		m.markAssistantBoundary(scope)
		return true

	case "content_block_start", "content_block_delta", "content_block_stop":
		// Skip partial streaming events — the full assistant message covers these.
		return true

	case "result":
		var ev struct {
			TotalCostUSD float64 `json:"total_cost_usd"`
			DurationMS   float64 `json:"duration_ms"`
			NumTurns     int     `json:"num_turns"`
			Usage        *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		json.Unmarshal([]byte(line), &ev)
		var parts []string
		if ev.TotalCostUSD > 0 {
			parts = append(parts, fmt.Sprintf("cost=$%.4f", ev.TotalCostUSD))
		}
		if ev.DurationMS > 0 {
			parts = append(parts, fmt.Sprintf("duration=%.1fs", ev.DurationMS/1000))
		}
		if ev.NumTurns > 0 {
			parts = append(parts, fmt.Sprintf("turns=%d", ev.NumTurns))
		}
		if ev.Usage != nil {
			parts = append(parts, fmt.Sprintf("in=%d out=%d", ev.Usage.InputTokens, ev.Usage.OutputTokens))
		}
		summary := "done"
		if len(parts) > 0 {
			summary = strings.Join(parts, " ")
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		return true

	default:
		return false
	}
}

// --- Event handling ---

func (m *Model) handleEvent(ev stream.ClaudeEvent) {
	scope := ""
	sessionID := 0
	if ev.TurnID != "" {
		if sid, err := strconv.Atoi(strings.TrimSpace(ev.TurnID)); err == nil && sid > 0 {
			sessionID = sid
			scope = m.sessionScope(sid)
			m.ensureSession(sid)
		}
	}
	if scope == "" && m.sessionID > 0 {
		scope = m.sessionScope(m.sessionID)
		sessionID = m.sessionID
	}
	m.switchStreamScope(scope)

	switch ev.Type {
	case "system":
		m.addScopedLine(scope, initLabelStyle.Render(
			fmt.Sprintf("[init] session=%s model=%s", ev.TurnID, ev.Model)))
		m.addSimplifiedLine(scope, dimStyle.Render("initialized"))
		if ev.Model != "" {
			m.modelName = ev.Model
			if sessionID > 0 {
				s := m.ensureSession(sessionID)
				s.Model = ev.Model
				s.LastUpdate = time.Now()
				m.setSessionAction(sessionID, "initialized")
			}
		}

	case "assistant":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				switch block.Type {
				case "text":
					text := block.Text
					if len(text) > 500 {
						text = text[:500] + "..."
					}
					m.addScopedLine(scope, textLabelStyle.Render("[text]"))
					m.addScopedLine(scope, "  "+textStyle.Render(text))
					m.recordAssistantText(scope, text)
					if sessionID > 0 {
						m.setSessionAction(sessionID, "responding")
					}
				case "tool_use":
					m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", block.Name)))
					m.recordToolCall(scope, block.Name)
					m.renderToolInput(scope, block.Name, string(block.Input))
					if sessionID > 0 {
						action := "running tool"
						if block.Name != "" {
							action = "running " + block.Name
						}
						m.setSessionAction(sessionID, action)
					}
				case "tool_result":
					m.addScopedLine(scope, toolResultStyle.Render("[tool_result]"))
					m.addSimplifiedLine(scope, dimStyle.Render("tool result"))
					if sessionID > 0 {
						m.setSessionAction(sessionID, "processing tool result")
					}
				case "thinking":
					text := block.Text
					if len(text) > 200 {
						text = text[:200] + "..."
					}
					m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
					m.addScopedLine(scope, "  "+thinkingTextStyle.Render(compactWhitespace(text)))
					if sessionID > 0 {
						m.setSessionAction(sessionID, "thinking")
					}
				}
			}
		}

	case "user":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				if block.Type == "tool_result" {
					m.markAssistantBoundary(scope)
					content := block.ToolContentText()
					if content == "" {
						m.addScopedLine(scope, dimStyle.Render("[empty result]"))
					} else {
						label := toolResultStyle.Render("[result]")
						if block.IsError {
							label = lipgloss.NewStyle().Foreground(theme.ColorRed).Render("[error]")
						}
						m.addScopedLine(scope, label)
						// Show first few lines of result, truncated.
						lines := strings.SplitN(content, "\n", 8)
						for i, line := range lines {
							if i >= 6 {
								m.addScopedLine(scope, dimStyle.Render("  ... (truncated)"))
								break
							}
							if len(line) > 200 {
								line = line[:200] + "..."
							}
							m.addScopedLine(scope, dimStyle.Render("  "+line))
						}
					}
				}
			}
		} else {
			m.addScopedLine(scope, dimStyle.Render("[tool response received]"))
		}
		if sessionID > 0 {
			m.setSessionAction(sessionID, "processing tool result")
		}

	case "content_block_start":
		m.flushStream()
		m.ensureToolInputBuf().Reset()
		if ev.ContentBlock != nil {
			m.currentBlockType = ev.ContentBlock.Type
			m.currentToolName = ev.ContentBlock.Name
			switch ev.ContentBlock.Type {
			case "thinking":
				m.addScopedLine(scope, thinkingLabelStyle.Render("[thinking]"))
				if sessionID > 0 {
					m.setSessionAction(sessionID, "thinking")
				}
			case "tool_use":
				m.addScopedLine(scope, toolLabelStyle.Render(fmt.Sprintf("[tool:%s]", ev.ContentBlock.Name)))
				m.recordToolCall(scope, ev.ContentBlock.Name)
				if sessionID > 0 {
					action := "running tool"
					if ev.ContentBlock.Name != "" {
						action = "running " + ev.ContentBlock.Name
					}
					m.setSessionAction(sessionID, action)
				}
			case "text":
				m.addScopedLine(scope, textLabelStyle.Render("[text]"))
				if sessionID > 0 {
					m.setSessionAction(sessionID, "responding")
				}
			}
		}

	case "content_block_delta":
		if ev.Delta != nil {
			if ev.Delta.PartialJSON != "" {
				// Accumulate tool JSON silently — will format on block stop.
				m.ensureToolInputBuf().WriteString(ev.Delta.PartialJSON)
			} else if ev.Delta.Text != "" {
				m.appendDelta(ev.Delta.Text)
			}
		}

	case "content_block_stop":
		m.flushStream()
		if m.currentBlockType == "tool_use" && m.toolInputBuf != nil && m.toolInputBuf.Len() > 0 {
			m.renderToolInput(scope, m.currentToolName, m.toolInputBuf.String())
		}
		m.addScopedLine(scope, "")
		m.currentBlockType = ""
		m.currentToolName = ""
		m.ensureToolInputBuf().Reset()
		if sessionID > 0 {
			m.setSessionAction(sessionID, "waiting")
		}

	case "result":
		var parts []string
		if ev.TotalCostUSD > 0 {
			parts = append(parts, fmt.Sprintf("cost=$%.4f", ev.TotalCostUSD))
			m.costUSD = ev.TotalCostUSD
		}
		if ev.DurationMS > 0 {
			parts = append(parts, fmt.Sprintf("duration=%.1fs", ev.DurationMS/1000))
		}
		if ev.NumTurns > 0 {
			parts = append(parts, fmt.Sprintf("turns=%d", ev.NumTurns))
			m.numTurns = ev.NumTurns
		}
		if ev.Usage != nil {
			parts = append(parts, fmt.Sprintf("in=%d out=%d",
				ev.Usage.InputTokens, ev.Usage.OutputTokens))
			m.inputTokens = ev.Usage.InputTokens
			m.outputTokens = ev.Usage.OutputTokens
		}
		summary := "done"
		if len(parts) > 0 {
			summary = strings.Join(parts, " ")
		}
		m.addScopedLine(scope, resultLabelStyle.Render("[result]")+" "+summary)
		m.addSimplifiedLine(scope, dimStyle.Render("result"))
		if sessionID > 0 {
			m.setSessionAction(sessionID, "turn complete")
		}

	case "message":
		m.addSimplifiedLine(scope, dimStyle.Render("message"))

	default:
		if ev.Type != "" {
			m.addScopedLine(scope, dimStyle.Render(fmt.Sprintf("[%s]", ev.Type)))
		}
	}
}

// --- View rendering ---

func (m Model) View() string {
	if m.width == 0 || m.height < 3 {
		return "Loading..."
	}

	panelHeight := m.height - 2
	if panelHeight < 1 {
		panelHeight = 1
	}

	header := m.renderHeader()
	statusBar := m.renderStatusBar()

	var panels string
	rightOuterW := m.width - leftPanelOuterWidth
	if rightOuterW >= 20 {
		// Two-column layout.
		left := m.renderLeftPanel(leftPanelOuterWidth, panelHeight)
		right := m.renderRightPanel(rightOuterW, panelHeight)
		panels = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		// Narrow terminal: right panel only.
		panels = m.renderRightPanel(m.width, panelHeight)
	}

	return header + "\n" + panels + "\n" + statusBar
}

func (m Model) renderHeader() string {
	var title string
	if m.loopName != "" {
		title = fmt.Sprintf(" adaf loop — %s — %s ", m.projectName, m.loopName)
	} else {
		title = fmt.Sprintf(" adaf run — %s ", m.projectName)
	}
	return headerStyle.
		Width(m.width).
		MaxWidth(m.width).
		Render(title)
}

func (m Model) renderStatusBar() string {
	var parts []string
	if m.focus == focusCommand {
		parts = append(parts, shortcut("j/k", "select"))
		parts = append(parts, shortcut("tab", "detail"))
	} else {
		parts = append(parts, shortcut("j/k", "scroll"))
		parts = append(parts, shortcut("pgup/dn", "page"))
		parts = append(parts, shortcut("tab", "command"))
	}
	parts = append(parts, shortcut("1-5", "views"))
	if m.leftSection == leftSectionAgents {
		parts = append(parts, shortcut("[/]", "agent"))
		if m.focus == focusDetail {
			parts = append(parts, shortcut("t/T", "layer"))
		}
	}

	total := m.totalLines()
	vh := m.detailViewportHeight()
	if total > vh {
		pct := 0
		ms := m.maxScroll()
		if ms > 0 {
			pct = m.scrollPos * 100 / ms
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("%d%%", pct)))
	}

	parts = append(parts, statusValueStyle.Render("view="+m.leftSectionLabel()))
	if m.sessionModeID > 0 {
		if m.live {
			parts = append(parts, statusValueStyle.Render("stream=live"))
		} else {
			parts = append(parts, statusValueStyle.Render("stream=syncing"))
		}
	}
	if m.leftSection == leftSectionAgents {
		parts = append(parts, statusValueStyle.Render("layer="+m.detailLayerLabel()))
	}
	if selected := m.selectedScope(); selected != "" {
		parts = append(parts, statusValueStyle.Render("detail="+selected))
	}
	if m.leftSection == leftSectionIssues && len(m.issues) > 0 {
		idx := m.selectedIssue
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.issues) {
			idx = len(m.issues) - 1
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("issue=#%d", m.issues[idx].ID)))
	}
	if m.leftSection == leftSectionDocs && len(m.docs) > 0 {
		idx := m.selectedDoc
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.docs) {
			idx = len(m.docs) - 1
		}
		parts = append(parts, statusValueStyle.Render("doc="+m.docs[idx].ID))
	}
	if m.leftSection == leftSectionPlan && m.plan != nil && len(m.plan.Phases) > 0 {
		idx := m.selectedPhase
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.plan.Phases) {
			idx = len(m.plan.Phases) - 1
		}
		phaseID := strings.TrimSpace(m.plan.Phases[idx].ID)
		if phaseID == "" {
			phaseID = fmt.Sprintf("%d", idx+1)
		}
		parts = append(parts, statusValueStyle.Render("phase="+phaseID))
	}
	if m.leftSection == leftSectionLogs && len(m.turns) > 0 {
		idx := m.selectedTurn
		if idx < 0 {
			idx = 0
		}
		if idx >= len(m.turns) {
			idx = len(m.turns) - 1
		}
		parts = append(parts, statusValueStyle.Render(fmt.Sprintf("turn=#%d", m.turns[idx].ID)))
	}

	if m.done {
		parts = append(parts, shortcut("esc", "back"))
		parts = append(parts, shortcut("q", "quit"))
	} else {
		if m.sessionModeID > 0 {
			parts = append(parts, shortcut("ctrl+d", "detach"))
		}
		parts = append(parts, shortcut("ctrl+c", "stop"))
	}

	content := strings.Join(parts, statusValueStyle.Render("  "))
	return statusBarStyle.
		Width(m.width).
		MaxWidth(m.width).
		Render(content)
}

func (m Model) leftSectionLabel() string {
	switch m.leftSection {
	case leftSectionIssues:
		return "issues"
	case leftSectionDocs:
		return "docs"
	case leftSectionPlan:
		return "plan"
	case leftSectionLogs:
		return "logs"
	default:
		return "agents"
	}
}

func shortcut(k, desc string) string {
	return statusKeyStyle.Render(k) + statusValueStyle.Render(" "+desc)
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "running", "awaiting_input", "waiting", "waiting_for_spawns":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case "completed", "merged":
		return lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case "failed", "rejected":
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	}
}

func issueStatusStyle(status string) lipgloss.Style {
	switch status {
	case "open":
		return lipgloss.NewStyle().Foreground(theme.ColorGreen).Bold(true)
	case "in_progress":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow).Bold(true)
	case "resolved":
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	case "wontfix":
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	}
}

func issuePriorityStyle(priority string) lipgloss.Style {
	switch priority {
	case "critical":
		return lipgloss.NewStyle().Foreground(theme.ColorRed).Bold(true)
	case "high":
		return lipgloss.NewStyle().Foreground(theme.ColorPeach).Bold(true)
	case "medium":
		return lipgloss.NewStyle().Foreground(theme.ColorYellow)
	case "low":
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorOverlay0)
	}
}

func formatTimeAgoShort(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}

func (m Model) commandEntries() []commandEntry {
	entries := []commandEntry{
		{
			scope:    "",
			title:    "All agents",
			status:   "running",
			action:   m.loopStepProfile,
			duration: m.elapsed.Round(time.Second).String(),
			depth:    0,
		},
	}
	if m.done {
		entries[0].status = "completed"
	}
	if entries[0].action == "" {
		entries[0].action = "monitoring"
	}

	spawns := append([]SpawnInfo(nil), m.spawns...)
	sort.Slice(spawns, func(i, j int) bool {
		return spawns[i].ID < spawns[j].ID
	})
	childrenByParent := make(map[string][]SpawnInfo, len(spawns))
	rootSpawns := make([]SpawnInfo, 0, len(spawns))
	for _, sp := range spawns {
		parentScope := ""
		if sp.ParentSpawnID > 0 {
			parentScope = m.spawnScope(sp.ParentSpawnID)
		} else if sp.ParentTurnID > 0 {
			parentScope = m.sessionScope(sp.ParentTurnID)
		}
		if parentScope == "" {
			rootSpawns = append(rootSpawns, sp)
			continue
		}
		childrenByParent[parentScope] = append(childrenByParent[parentScope], sp)
	}

	now := time.Now()
	appendSpawn := func(sp SpawnInfo, depth int, includeParentHint bool) {
		started := m.spawnFirstSeen[sp.ID]
		duration := ""
		if !started.IsZero() {
			d := now.Sub(started).Round(time.Second)
			if d < 0 {
				d = 0
			}
			duration = d.String()
		}
		title := fmt.Sprintf("#%d %s", sp.ID, sp.Profile)
		if role := strings.TrimSpace(sp.Role); role != "" {
			title += " as " + role
		}
		if includeParentHint && sp.ParentTurnID > 0 {
			title += fmt.Sprintf(" (turn #%d)", sp.ParentTurnID)
		}
		status := strings.TrimSpace(sp.Status)
		if status == "" {
			status = "running"
		}
		var actionParts []string
		if role := strings.TrimSpace(sp.Role); role != "" {
			actionParts = append(actionParts, "role="+role)
		}
		if sp.Status == "awaiting_input" || strings.TrimSpace(sp.Question) != "" {
			actionParts = append(actionParts, "awaiting input")
		}
		if includeParentHint && sp.ParentTurnID > 0 {
			actionParts = append(actionParts, fmt.Sprintf("parent turn #%d", sp.ParentTurnID))
		}
		if len(actionParts) == 0 {
			actionParts = append(actionParts, "delegated")
		}
		entries = append(entries, commandEntry{
			scope:    m.spawnScope(sp.ID),
			title:    title,
			status:   status,
			action:   strings.Join(actionParts, " · "),
			duration: duration,
			depth:    depth,
		})
	}
	seenSpawns := make(map[int]struct{}, len(spawns))
	var appendSpawnTree func(parentScope string, depth int)
	appendSpawnTree = func(parentScope string, depth int) {
		children := childrenByParent[parentScope]
		for _, sp := range children {
			if _, seen := seenSpawns[sp.ID]; seen {
				continue
			}
			seenSpawns[sp.ID] = struct{}{}
			appendSpawn(sp, depth, false)
			appendSpawnTree(m.spawnScope(sp.ID), depth+1)
		}
	}

	for _, sid := range m.sessionOrder {
		s := m.sessions[sid]
		if s == nil {
			continue
		}
		status := strings.TrimSpace(s.Status)
		if status == "" {
			status = "running"
		}
		title := fmt.Sprintf("turn #%d %s", s.ID, s.Agent)
		if s.Profile != "" {
			title = fmt.Sprintf("turn #%d %s (%s)", s.ID, s.Profile, s.Agent)
		}
		if s.Model != "" {
			title += " · " + s.Model
		}
		duration := "0s"
		if !s.StartedAt.IsZero() {
			end := now
			if !s.EndedAt.IsZero() {
				end = s.EndedAt
			}
			d := end.Sub(s.StartedAt).Round(time.Second)
			if d < 0 {
				d = 0
			}
			duration = d.String()
		}
		action := s.Action
		if action == "" {
			action = "idle"
		}
		entries = append(entries, commandEntry{
			scope:    m.sessionScope(s.ID),
			title:    title,
			status:   status,
			action:   action,
			duration: duration,
			depth:    0,
		})
		appendSpawnTree(m.sessionScope(s.ID), 1)
	}

	for _, sp := range rootSpawns {
		if _, seen := seenSpawns[sp.ID]; seen {
			continue
		}
		seenSpawns[sp.ID] = struct{}{}
		appendSpawn(sp, 0, sp.ParentTurnID > 0)
		appendSpawnTree(m.spawnScope(sp.ID), 1)
	}

	for _, sp := range spawns {
		if _, seen := seenSpawns[sp.ID]; seen {
			continue
		}
		seenSpawns[sp.ID] = struct{}{}
		appendSpawn(sp, 0, true)
		appendSpawnTree(m.spawnScope(sp.ID), 1)
	}

	return entries
}

func (m Model) renderLeftPanel(outerW, outerH int) string {
	hf, vf := leftPanelStyle.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionTitleStyle.Render("Command Center"))
	if m.focus == focusCommand {
		lines = append(lines, dimStyle.Render("focus: left panel"))
	} else {
		lines = append(lines, dimStyle.Render("focus: detail"))
	}
	helpLine := "tab focus · 1-5 views"
	if m.leftSection == leftSectionAgents {
		helpLine += " · t/T detail layer"
	}
	lines = append(lines, dimStyle.Render(helpLine))
	if m.leftSection == leftSectionAgents {
		lines = append(lines, dimStyle.Render("[/] cycle running agents"))
	} else {
		lines = append(lines, dimStyle.Render("j/k select entry"))
	}
	lines = append(lines, "")

	lines = append(lines, fieldLine("Agent", m.agentName))
	lines = append(lines, fieldLine("Elapsed", m.elapsed.Round(time.Second).String()))
	if m.loopName != "" {
		lines = append(lines, fieldLine("Loop", m.loopName))
	}
	if m.loopTotalSteps > 0 {
		lines = append(lines, fieldLine("Step", fmt.Sprintf("%d/%d", m.loopStep+1, m.loopTotalSteps)))
	}
	if m.loopStepProfile != "" {
		lines = append(lines, fieldLine("Profile", m.loopStepProfile))
	}
	if m.activeLoop != nil && m.activeLoop.Status == "running" {
		runLabel := fmt.Sprintf("#%d %s", m.activeLoop.ID, m.activeLoop.Status)
		if m.activeLoop.HexID != "" {
			runLabel = fmt.Sprintf("#%d [%s] %s", m.activeLoop.ID, m.activeLoop.HexID, m.activeLoop.Status)
		}
		lines = append(lines, fieldLine("Loop Run", runLabel))
	}
	lines = append(lines, "")

	entries := m.commandEntries()
	lines = append(lines, sectionTitleStyle.Render("Views"))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionAgents, fmt.Sprintf("1 Agents (%d)", len(entries))))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionIssues, fmt.Sprintf("2 Issues (%d)", len(m.issues))))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionDocs, fmt.Sprintf("3 Docs (%d)", len(m.docs))))
	planCount := 0
	if m.plan != nil {
		planCount = len(m.plan.Phases)
	}
	lines = append(lines, leftViewChip(m.leftSection == leftSectionPlan, fmt.Sprintf("4 Plan (%d)", planCount)))
	lines = append(lines, leftViewChip(m.leftSection == leftSectionLogs, fmt.Sprintf("5 Logs (%d)", len(m.turns))))
	lines = append(lines, "")

	switch m.leftSection {
	case leftSectionIssues:
		cursorLine = m.appendIssuesList(&lines, cw)
	case leftSectionDocs:
		cursorLine = m.appendDocsList(&lines, cw)
	case leftSectionPlan:
		cursorLine = m.appendPlanList(&lines, cw)
	case leftSectionLogs:
		cursorLine = m.appendLogsList(&lines, cw)
	default:
		cursorLine = m.appendAgentsList(&lines, cw, entries)
	}
	lines = append(lines, "")

	lines = append(lines, sectionTitleStyle.Render("Usage"))
	usage := fmt.Sprintf("in=%d out=%d", m.inputTokens, m.outputTokens)
	if m.costUSD > 0 {
		usage += fmt.Sprintf(" cost=$%.4f", m.costUSD)
	}
	if m.numTurns > 0 {
		usage += fmt.Sprintf(" turns=%d", m.numTurns)
	}
	lines = append(lines, dimStyle.Render(truncate(usage, cw)))

	content := fitToSizeWithCursor(lines, cw, ch, cursorLine)
	return leftPanelStyle.Render(content)
}

func leftViewChip(active bool, text string) string {
	if active {
		return lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal).Render("> " + text)
	}
	return dimStyle.Render("  " + text)
}

func hierarchyPrefix(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("|  ", depth-1) + "+- "
}

func (m Model) appendAgentsList(lines *[]string, cw int, entries []commandEntry) int {
	*lines = append(*lines, sectionTitleStyle.Render("Agents"))
	if len(entries) == 0 {
		*lines = append(*lines, dimStyle.Render("  no active entries"))
		return -1
	}
	cursorLine := -1
	selected := m.selectedEntry
	if selected < 0 {
		selected = 0
	}
	if selected >= len(entries) {
		selected = len(entries) - 1
	}
	for i, entry := range entries {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		status := statusStyle(entry.status).Render(entry.status)
		title := entry.title
		if entry.depth > 0 {
			title = hierarchyPrefix(entry.depth) + title
		}
		line := fmt.Sprintf("%s%s [%s]", prefix, titleStyle.Render(truncate(title, cw-8)), status)
		*lines = append(*lines, line)
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		metaPrefix := "   "
		if entry.depth > 0 {
			metaPrefix += strings.Repeat("  ", entry.depth)
		}
		maxMetaWidth := cw - len(metaPrefix)
		if maxMetaWidth < 1 {
			maxMetaWidth = 1
		}
		meta := dimStyle.Render(metaPrefix + truncate(entry.duration+" · "+entry.action, maxMetaWidth))
		*lines = append(*lines, meta)
	}
	return cursorLine
}

func (m Model) appendIssuesList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Issues"))
	if len(m.issues) == 0 {
		*lines = append(*lines, dimStyle.Render("  no issues recorded"))
		return -1
	}
	cursorLine := -1
	selected := m.selectedIssue
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.issues) {
		selected = len(m.issues) - 1
	}
	for i, issue := range m.issues {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		title := fmt.Sprintf("#%d %s", issue.ID, truncate(issue.Title, cw-8))
		*lines = append(*lines, prefix+titleStyle.Render(title))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		meta := fmt.Sprintf("   %s · %s",
			issuePriorityStyle(issue.Priority).Render(issue.Priority),
			issueStatusStyle(issue.Status).Render(issue.Status))
		*lines = append(*lines, truncate(meta, cw))
	}
	return cursorLine
}

func (m Model) appendDocsList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Docs"))
	if len(m.docs) == 0 {
		*lines = append(*lines, dimStyle.Render("  no docs recorded"))
		return -1
	}
	cursorLine := -1
	selected := m.selectedDoc
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.docs) {
		selected = len(m.docs) - 1
	}
	for i, doc := range m.docs {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		title := fmt.Sprintf("[%s] %s", doc.ID, truncate(doc.Title, cw-8))
		*lines = append(*lines, prefix+titleStyle.Render(title))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		meta := fmt.Sprintf("   %s · %d chars", formatTimeAgoShort(doc.Updated), len(doc.Content))
		*lines = append(*lines, dimStyle.Render(truncate(meta, cw)))
	}
	return cursorLine
}

func (m Model) appendPlanList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Plan"))
	if m.plan == nil {
		*lines = append(*lines, dimStyle.Render("  no active plan"))
		return -1
	}

	title := strings.TrimSpace(m.plan.Title)
	if title == "" {
		title = "(untitled)"
	}
	status := strings.TrimSpace(m.plan.Status)
	if status == "" {
		status = "active"
	}
	*lines = append(*lines, valueStyle.Render("  "+truncate(m.plan.ID+" · "+title, cw-2)))
	*lines = append(*lines, dimStyle.Render("  status: "+status))
	if len(m.plan.Phases) == 0 {
		*lines = append(*lines, dimStyle.Render("  no phases"))
		return -1
	}

	*lines = append(*lines, "")
	*lines = append(*lines, sectionTitleStyle.Render("Phases"))
	selected := m.selectedPhase
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.plan.Phases) {
		selected = len(m.plan.Phases) - 1
	}
	cursorLine := -1
	for i, phase := range m.plan.Phases {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		indicator := theme.PhaseStatusIndicator(phase.Status)
		name := phase.Title
		if strings.TrimSpace(name) == "" {
			name = phase.ID
		}
		*lines = append(*lines, prefix+titleStyle.Render(indicator+truncate(name, cw-8)))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		metaID := strings.TrimSpace(phase.ID)
		if metaID == "" {
			metaID = fmt.Sprintf("phase-%d", i+1)
		}
		meta := fmt.Sprintf("   %s · %s", metaID, phase.Status)
		*lines = append(*lines, dimStyle.Render(truncate(meta, cw)))
	}
	return cursorLine
}

func (m Model) appendLogsList(lines *[]string, cw int) int {
	*lines = append(*lines, sectionTitleStyle.Render("Logs"))
	if len(m.turns) == 0 {
		*lines = append(*lines, dimStyle.Render("  no turn logs yet"))
		return -1
	}
	selected := m.selectedTurn
	if selected < 0 {
		selected = 0
	}
	if selected >= len(m.turns) {
		selected = len(m.turns) - 1
	}
	cursorLine := -1
	for i, turn := range m.turns {
		prefix := "  "
		titleStyle := valueStyle
		if i == selected {
			prefix = "> "
			titleStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorTeal)
		}
		profile := strings.TrimSpace(turn.ProfileName)
		if profile == "" {
			profile = strings.TrimSpace(turn.Agent)
		}
		if profile == "" {
			profile = "turn"
		}
		title := fmt.Sprintf("#%d %s", turn.ID, profile)
		*lines = append(*lines, prefix+titleStyle.Render(truncate(title, cw-8)))
		if i == selected {
			cursorLine = len(*lines) - 1
		}
		meta := fmt.Sprintf("   %s · %s", turn.Date.Format("2006-01-02 15:04"), truncate(compactWhitespace(turn.Objective), cw-26))
		*lines = append(*lines, dimStyle.Render(truncate(meta, cw)))
	}
	return cursorLine
}

func fieldLine(label, value string) string {
	return labelStyle.Render(label) + valueStyle.Render(value)
}

func (m Model) renderRightPanel(outerW, outerH int) string {
	hf, vf := rightPanelStyle.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	headerLines := m.detailHeaderLines()
	viewportHeight := ch - len(headerLines)
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	visible := m.getVisibleLines(viewportHeight, cw)
	combined := make([]string, 0, len(headerLines)+len(visible))
	combined = append(combined, headerLines...)
	combined = append(combined, visible...)
	content := fitToSize(combined, cw, ch)
	return rightPanelStyle.Render(content)
}

// getVisibleLines returns the slice of lines visible in the viewport,
// including a partial streaming line if present.
func (m Model) getVisibleLines(height, width int) []string {
	wrapped := m.detailLines(width)
	total := len(wrapped)
	if total == 0 {
		return nil
	}

	start := m.scrollPos
	if start < 0 {
		start = 0
	}
	if start >= total {
		start = total - 1
	}

	end := start + height
	if end > total {
		end = total
	}

	return wrapped[start:end]
}

func (m Model) detailLines(width int) []string {
	switch m.leftSection {
	case leftSectionIssues:
		return wrapRenderableLines(m.issueDetailLines(), width)
	case leftSectionDocs:
		return wrapRenderableLines(m.docDetailLines(), width)
	case leftSectionPlan:
		return wrapRenderableLines(m.planDetailLines(), width)
	case leftSectionLogs:
		return wrapRenderableLines(m.logDetailLines(), width)
	}

	var lines []string
	switch m.detailLayer {
	case detailLayerSimplified:
		lines = m.simplifiedDetailLines()
	case detailLayerPrompt:
		lines = m.promptDetailLines()
	case detailLayerLastMessage:
		lines = m.finalMessageDetailLines()
	case detailLayerActivity:
		lines = m.activityDetailLines()
	default:
		lines = m.rawDetailLines()
	}
	return wrapRenderableLines(lines, width)
}

func (m Model) issueDetailLines() []string {
	if len(m.issues) == 0 {
		return []string{
			sectionTitleStyle.Render("Issues"),
			"",
			dimStyle.Render("No issues available."),
		}
	}

	idx := m.selectedIssue
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.issues) {
		idx = len(m.issues) - 1
	}
	issue := m.issues[idx]

	lines := []string{
		sectionTitleStyle.Render(fmt.Sprintf("Issue #%d", issue.ID)),
		fieldLine("Status", issueStatusStyle(issue.Status).Render(issue.Status)),
		fieldLine("Priority", issuePriorityStyle(issue.Priority).Render(issue.Priority)),
		fieldLine("Created", issue.Created.Format("2006-01-02 15:04")),
		fieldLine("Updated", issue.Updated.Format("2006-01-02 15:04")),
	}
	if issue.TurnID > 0 {
		lines = append(lines, fieldLine("Turn", fmt.Sprintf("#%d", issue.TurnID)))
	}
	if len(issue.Labels) > 0 {
		lines = append(lines, fieldLine("Labels", strings.Join(issue.Labels, ", ")))
	}
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Title"))
	lines = append(lines, textStyle.Render(issue.Title))
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Description"))
	if strings.TrimSpace(issue.Description) == "" {
		lines = append(lines, dimStyle.Render("No description."))
		return lines
	}
	for _, line := range splitRenderableLines(issue.Description) {
		lines = append(lines, textStyle.Render(line))
	}
	return lines
}

func (m Model) docDetailLines() []string {
	if len(m.docs) == 0 {
		return []string{
			sectionTitleStyle.Render("Docs"),
			"",
			dimStyle.Render("No documents available."),
		}
	}

	idx := m.selectedDoc
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.docs) {
		idx = len(m.docs) - 1
	}
	doc := m.docs[idx]

	lines := []string{
		sectionTitleStyle.Render(fmt.Sprintf("Doc %s", doc.ID)),
		fieldLine("Title", doc.Title),
		fieldLine("Updated", doc.Updated.Format("2006-01-02 15:04")),
		fieldLine("Size", fmt.Sprintf("%d chars", len(doc.Content))),
		"",
		sectionTitleStyle.Render("Content"),
	}
	if strings.TrimSpace(doc.Content) == "" {
		lines = append(lines, dimStyle.Render("Empty content."))
		return lines
	}
	for _, line := range splitRenderableLines(doc.Content) {
		lines = append(lines, textStyle.Render(line))
	}
	return lines
}

func (m Model) planDetailLines() []string {
	if m.plan == nil {
		return []string{
			sectionTitleStyle.Render("Plan"),
			"",
			dimStyle.Render("No active plan available."),
		}
	}

	status := strings.TrimSpace(m.plan.Status)
	if status == "" {
		status = "active"
	}
	title := strings.TrimSpace(m.plan.Title)
	if title == "" {
		title = "(untitled)"
	}
	lines := []string{
		sectionTitleStyle.Render("Plan " + m.plan.ID),
		fieldLine("Status", status),
		fieldLine("Updated", m.plan.Updated.Format("2006-01-02 15:04")),
		fieldLine("Phases", fmt.Sprintf("%d", len(m.plan.Phases))),
		"",
		sectionTitleStyle.Render("Title"),
		textStyle.Render(title),
	}
	if strings.TrimSpace(m.plan.Description) != "" {
		lines = append(lines, "")
		lines = append(lines, sectionTitleStyle.Render("Description"))
		for _, line := range splitRenderableLines(m.plan.Description) {
			lines = append(lines, textStyle.Render(line))
		}
	}

	if len(m.plan.Phases) == 0 {
		lines = append(lines, "", dimStyle.Render("No phases defined."))
		return lines
	}

	idx := m.selectedPhase
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.plan.Phases) {
		idx = len(m.plan.Phases) - 1
	}
	phase := m.plan.Phases[idx]
	phaseID := strings.TrimSpace(phase.ID)
	if phaseID == "" {
		phaseID = fmt.Sprintf("phase-%d", idx+1)
	}
	phaseTitle := strings.TrimSpace(phase.Title)
	if phaseTitle == "" {
		phaseTitle = "(untitled phase)"
	}

	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render(fmt.Sprintf("Phase %d/%d", idx+1, len(m.plan.Phases))))
	lines = append(lines, fieldLine("ID", phaseID))
	lines = append(lines, fieldLine("Status", phase.Status))
	lines = append(lines, fieldLine("Priority", fmt.Sprintf("%d", phase.Priority)))
	if len(phase.DependsOn) > 0 {
		lines = append(lines, fieldLine("Depends", strings.Join(phase.DependsOn, ", ")))
	}
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Phase Title"))
	lines = append(lines, textStyle.Render(phaseTitle))
	lines = append(lines, "")
	lines = append(lines, sectionTitleStyle.Render("Phase Description"))
	if strings.TrimSpace(phase.Description) == "" {
		lines = append(lines, dimStyle.Render("No phase description."))
	} else {
		for _, line := range splitRenderableLines(phase.Description) {
			lines = append(lines, textStyle.Render(line))
		}
	}
	return lines
}

func (m Model) logDetailLines() []string {
	if len(m.turns) == 0 {
		return []string{
			sectionTitleStyle.Render("Logs"),
			"",
			dimStyle.Render("No turn logs captured yet."),
		}
	}

	idx := m.selectedTurn
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.turns) {
		idx = len(m.turns) - 1
	}
	turn := m.turns[idx]

	profile := strings.TrimSpace(turn.ProfileName)
	if profile == "" {
		profile = "(unknown)"
	}
	agentLabel := strings.TrimSpace(turn.Agent)
	if strings.TrimSpace(turn.AgentModel) != "" {
		agentLabel += " · " + strings.TrimSpace(turn.AgentModel)
	}
	if strings.TrimSpace(turn.HexID) != "" {
		agentLabel += " [" + strings.TrimSpace(turn.HexID) + "]"
	}

	lines := []string{
		sectionTitleStyle.Render(fmt.Sprintf("Turn #%d", turn.ID)),
		fieldLine("Date", turn.Date.Format("2006-01-02 15:04:05")),
		fieldLine("Profile", profile),
		fieldLine("Agent", agentLabel),
		fieldLine("Duration", fmt.Sprintf("%ds", turn.DurationSecs)),
	}
	if strings.TrimSpace(turn.PlanID) != "" {
		lines = append(lines, fieldLine("Plan", turn.PlanID))
	}
	if strings.TrimSpace(turn.LoopRunHexID) != "" {
		lines = append(lines, fieldLine("Run", turn.LoopRunHexID))
	}
	if strings.TrimSpace(turn.StepHexID) != "" {
		lines = append(lines, fieldLine("Step", turn.StepHexID))
	}
	if strings.TrimSpace(turn.CommitHash) != "" {
		lines = append(lines, fieldLine("Commit", turn.CommitHash))
	}

	appendSection := func(title, body string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		lines = append(lines, "")
		lines = append(lines, sectionTitleStyle.Render(title))
		for _, line := range splitRenderableLines(body) {
			lines = append(lines, textStyle.Render(line))
		}
	}

	appendSection("Objective", turn.Objective)
	appendSection("What Was Built", turn.WhatWasBuilt)
	appendSection("Key Decisions", turn.KeyDecisions)
	appendSection("Challenges", turn.Challenges)
	appendSection("Current State", turn.CurrentState)
	appendSection("Known Issues", turn.KnownIssues)
	appendSection("Next Steps", turn.NextSteps)
	appendSection("Build State", turn.BuildState)

	if len(lines) == 0 {
		return []string{dimStyle.Render("No log details available.")}
	}
	return lines
}

// --- Utility ---

// fitToSize takes a slice of styled lines and produces a string that is
// exactly w columns wide and h lines tall. Lines wider than w are truncated;
// shorter lines are right-padded with spaces; missing lines are blank.
func fitToSize(lines []string, w, h int) string {
	emptyLine := strings.Repeat(" ", w)
	result := make([]string, h)

	for i := 0; i < h; i++ {
		if i < len(lines) {
			line := lines[i]
			parts := splitRenderableLines(line)
			if len(parts) > 0 {
				line = parts[0]
			}
			line = ansi.Truncate(line, w, "")
			lw := lipgloss.Width(line)
			pad := w - lw
			if pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			result[i] = line
		} else {
			result[i] = emptyLine
		}
	}
	return strings.Join(result, "\n")
}

func fitToSizeWithCursor(lines []string, w, h, cursorLine int) string {
	if cursorLine < 0 || len(lines) <= h {
		return fitToSize(lines, w, h)
	}
	start := cursorLine - (h / 2)
	if start < 0 {
		start = 0
	}
	maxStart := len(lines) - h
	if start > maxStart {
		start = maxStart
	}
	end := start + h
	if end > len(lines) {
		end = len(lines)
	}
	return fitToSize(lines[start:end], w, h)
}

func wrapRenderableLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if width < 1 {
		width = 1
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, part := range splitRenderableLines(line) {
			wrapped := ansi.Wrap(part, width, " ")
			out = append(out, splitRenderableLines(wrapped)...)
		}
	}
	return out
}

func compactWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}
