package runtui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/store"
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
