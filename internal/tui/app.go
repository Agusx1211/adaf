package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

// appState distinguishes the modes of the unified TUI.
type appState int

const (
	stateSelector appState = iota
	stateRunning
	statePlanMenu                 // manage plans (switch/create/status/delete)
	statePlanCreateID             // text input for new plan ID
	statePlanCreateTitle          // text input for new plan title
	stateProfileName              // text input for new profile name
	stateProfileAgent             // pick agent for new profile
	stateProfileModel             // pick model for new profile
	stateProfileReasoning         // pick reasoning level for new profile
	stateProfileRole              // pick role for new profile
	stateProfileIntel             // input intelligence rating (1-10)
	stateProfileDesc              // input description text
	stateProfileMaxInst           // input max concurrent instances
	stateProfileSpawnable         // multi-select spawnable profiles
	stateProfileMaxPar            // input max parallel
	stateProfileMenu              // edit profile: field picker menu
	stateLoopName                 // text input for loop name
	stateLoopStepList             // list of steps, add/edit/remove/reorder
	stateLoopStepProfile          // pick profile for a step
	stateLoopStepTurns            // input turns for a step
	stateLoopStepInstr            // input custom instructions for a step
	stateLoopStepTools            // multi-select tools (stop, message, pushover)
	stateLoopMenu                 // edit loop: field picker menu
	stateSettings                 // settings screen (pushover credentials, etc.)
	stateSettingsPushoverUserKey  // input pushover user key
	stateSettingsPushoverAppToken // input pushover app token
	stateSessionPicker            // choose which active session to attach
	stateConfirmDelete            // confirmation before deleting a profile/loop
)

type selectorRefreshMsg struct{}

func selectorRefreshTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return selectorRefreshMsg{}
	})
}

// AppModel is the top-level bubbletea model for the unified adaf TUI.
type AppModel struct {
	store *store.Store
	state appState

	width  int
	height int

	// Global config (profiles live here).
	globalCfg *config.GlobalConfig

	// Selector state.
	profiles []profileEntry
	selected int
	plans    []store.Plan
	planSel  int

	// Plan creation state.
	planCreateIDInput    string
	planCreateTitleInput string
	planActionMsg        string
	selectorMsg          string
	sessionPickSel       int

	// Profile creation/editing wizard state.
	profileEditing           bool   // true = editing existing, false = creating new
	profileEditName          string // original name of profile being edited
	profileNameInput         string
	profileAgents            []string
	profileAgentSel          int
	profileModels            []string
	profileModelSel          int
	profileCustomModel       string
	profileCustomModelMode   bool
	profileSelectedModel     string
	profileReasoningLevels   []agentmeta.ReasoningLevel
	profileReasoningLevelSel int
	profileSelectedReasoning string
	profileRoleSel           int
	profileIntelInput        string
	profileDescInput         string
	profileMenuSel           int
	profileMaxInstInput      string
	profileSpawnableOptions  []string // profile names available for selection
	profileSpawnableSelected map[int]bool
	profileSpawnableSel      int
	profileMaxParInput       string

	// Loop creation/editing wizard state.
	loopEditing         bool
	loopEditName        string
	loopNameInput       string
	loopSteps           []config.LoopStep
	loopStepSel         int
	loopStepEditIdx     int // which step is being edited (-1 = adding new)
	loopStepProfileOpts []string
	loopStepProfileSel  int
	loopStepTurnsInput  string
	loopStepInstrInput  string
	loopStepCanStop     bool
	loopStepCanMsg      bool
	loopStepCanPushover bool
	loopStepToolsSel    int // cursor position in the tools multi-select
	loopMenuSel         int

	// Confirm delete state.
	confirmDeleteIdx int // index of profile/loop pending deletion

	// Settings screen state.
	settingsSel              int    // cursor in settings menu
	settingsPushoverUserKey  string // input buffer for pushover user key
	settingsPushoverAppToken string // input buffer for pushover app token

	// Cached project data for the selector.
	project        *store.ProjectConfig
	plan           *store.Plan
	issues         []store.Issue
	docs           []store.Doc
	logs           []store.SessionLog
	activeSessions []session.SessionMeta
	activeLoop     *store.LoopRun

	// Cached stats for selector display.
	cachedProfileStats map[string]*store.ProfileStats
	cachedLoopStats    map[string]*store.LoopStats

	// Running state — embedded run TUI model.
	runModel   runtui.Model
	runCancel  context.CancelFunc
	runEventCh chan any

	// Session mode: when non-nil, the agent is running via a session daemon.
	sessionClient *session.Client
	sessionID     int
}

// NewApp creates the unified TUI app model.
func NewApp(s *store.Store) AppModel {
	globalCfg, _ := config.Load()
	if ensureDefaultProfiles(globalCfg) {
		config.Save(globalCfg)
	}

	m := AppModel{
		store:     s,
		state:     stateSelector,
		globalCfg: globalCfg,
	}
	m.loadProjectData()
	m.rebuildProfiles()
	return m
}

func (m *AppModel) loadProjectData() {
	m.project, _ = m.store.LoadProject()
	m.plans, _ = m.store.ListPlans()
	m.project, _ = m.store.LoadProject() // refresh after potential lazy migration
	m.plan, _ = m.store.ActivePlan()
	activePlanID := ""
	if m.project != nil {
		activePlanID = strings.TrimSpace(m.project.ActivePlanID)
	}
	if m.plan != nil {
		status := strings.TrimSpace(m.plan.Status)
		if status != "" && status != "active" {
			m.plan = nil
			activePlanID = ""
		}
	}
	if activePlanID != "" {
		m.issues, _ = m.store.ListIssuesForPlan(activePlanID)
		m.docs, _ = m.store.ListDocsForPlan(activePlanID)
	} else {
		m.issues, _ = m.store.ListSharedIssues()
		m.docs, _ = m.store.ListSharedDocs()
	}
	m.logs, _ = m.store.ListLogs()
	m.loadStats()
	m.loadRuntimeData()
}

func (m *AppModel) loadRuntimeData() {
	if !session.IsAgentContext() {
		m.activeSessions, _ = session.ListActiveSessions()
	}
	m.activeLoop, _ = m.store.ActiveLoopRun()
}

func (m *AppModel) loadStats() {
	m.cachedProfileStats = make(map[string]*store.ProfileStats)
	m.cachedLoopStats = make(map[string]*store.LoopStats)
	profileStats, _ := m.store.ListProfileStats()
	for i := range profileStats {
		m.cachedProfileStats[profileStats[i].ProfileName] = &profileStats[i]
	}
	loopStats, _ := m.store.ListLoopStats()
	for i := range loopStats {
		m.cachedLoopStats[loopStats[i].LoopName] = &loopStats[i]
	}
}

func (m *AppModel) rebuildProfiles() {
	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())
	agent.PopulateFromConfig(agentsCfg)
	m.profiles = buildProfileList(m.globalCfg, agentsCfg)
}

// Init implements tea.Model.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		tea.SetWindowTitle("adaf"),
		selectorRefreshTick(),
	)
}

// Update implements tea.Model.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == stateRunning {
			updated, cmd := m.runModel.Update(msg)
			m.runModel = updated.(runtui.Model)
			return m, cmd
		}
		return m, nil
	case selectorRefreshMsg:
		if m.state == stateSelector {
			m.loadProjectData()
		}
		return m, selectorRefreshTick()
	}

	switch m.state {
	case stateSelector:
		return m.updateSelector(msg)
	case stateRunning:
		return m.updateRunning(msg)
	case statePlanMenu:
		return m.updatePlanMenu(msg)
	case statePlanCreateID:
		return m.updatePlanCreateID(msg)
	case statePlanCreateTitle:
		return m.updatePlanCreateTitle(msg)
	case stateProfileName:
		return m.updateProfileName(msg)
	case stateProfileAgent:
		return m.updateProfileAgent(msg)
	case stateProfileModel:
		return m.updateProfileModel(msg)
	case stateProfileReasoning:
		return m.updateProfileReasoning(msg)
	case stateProfileRole:
		return m.updateProfileRole(msg)
	case stateProfileIntel:
		return m.updateProfileIntel(msg)
	case stateProfileDesc:
		return m.updateProfileDesc(msg)
	case stateProfileMaxInst:
		return m.updateProfileMaxInst(msg)
	case stateProfileSpawnable:
		return m.updateProfileSpawnable(msg)
	case stateProfileMaxPar:
		return m.updateProfileMaxPar(msg)
	case stateProfileMenu:
		return m.updateProfileMenu(msg)
	case stateLoopName:
		return m.updateLoopName(msg)
	case stateLoopStepList:
		return m.updateLoopStepList(msg)
	case stateLoopStepProfile:
		return m.updateLoopStepProfile(msg)
	case stateLoopStepTurns:
		return m.updateLoopStepTurns(msg)
	case stateLoopStepInstr:
		return m.updateLoopStepInstr(msg)
	case stateLoopStepTools:
		return m.updateLoopStepTools(msg)
	case stateLoopMenu:
		return m.updateLoopMenu(msg)
	case stateSettings:
		return m.updateSettings(msg)
	case stateSettingsPushoverUserKey:
		return m.updateSettingsPushoverUserKey(msg)
	case stateSettingsPushoverAppToken:
		return m.updateSettingsPushoverAppToken(msg)
	case stateSessionPicker:
		return m.updateSessionPicker(msg)
	case stateConfirmDelete:
		return m.updateConfirmDelete(msg)
	}
	return m, nil
}

func (m AppModel) updateSelector(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if len(m.profiles) > 0 {
				m.selected = (m.selected + 1) % len(m.profiles)
				// Skip separators.
				if m.profiles[m.selected].IsSeparator {
					m.selected = (m.selected + 1) % len(m.profiles)
				}
			}
		case "k", "up":
			if len(m.profiles) > 0 {
				m.selected = (m.selected - 1 + len(m.profiles)) % len(m.profiles)
				// Skip separators.
				if m.profiles[m.selected].IsSeparator {
					m.selected = (m.selected - 1 + len(m.profiles)) % len(m.profiles)
				}
			}
		case "enter":
			if len(m.profiles) > 0 {
				return m.startAgent()
			}
		case "n":
			// Start new profile creation.
			m.profileEditing = false
			m.profileEditName = ""
			m.profileNameInput = ""
			m.state = stateProfileName
			return m, nil
		case "l":
			// Start new loop creation.
			m.loopEditing = false
			m.loopEditName = ""
			m.loopNameInput = ""
			m.loopSteps = nil
			m.state = stateLoopName
			return m, nil
		case "e":
			// Edit selected profile or loop.
			if m.selected >= 0 && m.selected < len(m.profiles) {
				p := m.profiles[m.selected]
				if p.IsLoop {
					return m.startEditLoop()
				}
				if !p.IsNew && !p.IsNewLoop && !p.IsSeparator {
					return m.startEditProfile()
				}
			}
			return m, nil
		case "d":
			// Delete selected profile or loop (unless sentinel/separator).
			if m.selected >= 0 && m.selected < len(m.profiles) {
				p := m.profiles[m.selected]
				if p.IsLoop || (!p.IsNew && !p.IsNewLoop && !p.IsSeparator) {
					m.confirmDeleteIdx = m.selected
					m.state = stateConfirmDelete
				}
			}
			return m, nil
		case "s":
			// Show and attach to active sessions.
			return m.showSessions()
		case "S":
			// Open settings.
			m.settingsSel = 0
			m.settingsPushoverUserKey = m.globalCfg.Pushover.UserKey
			m.settingsPushoverAppToken = m.globalCfg.Pushover.AppToken
			m.state = stateSettings
			return m, nil
		case "p":
			return m.openPlanManager()
		case "[":
			if err := m.cycleActivePlan(-1); err == nil {
				m.loadProjectData()
			}
			return m, nil
		case "]":
			if err := m.cycleActivePlan(1); err == nil {
				m.loadProjectData()
			}
			return m, nil
		}
	}
	return m, nil
}

// clampSelected ensures the selected index is valid after list changes.
func (m *AppModel) clampSelected() {
	if m.selected >= len(m.profiles) {
		m.selected = len(m.profiles) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	// Skip separators.
	if m.selected < len(m.profiles) && m.profiles[m.selected].IsSeparator {
		if m.selected > 0 {
			m.selected--
		} else {
			m.selected++
		}
	}
}

// updateConfirmDelete handles the y/n confirmation before deleting.
func (m AppModel) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y":
			if m.confirmDeleteIdx >= 0 && m.confirmDeleteIdx < len(m.profiles) {
				p := m.profiles[m.confirmDeleteIdx]
				if p.IsLoop {
					m.globalCfg.RemoveLoop(p.LoopName)
					config.Save(m.globalCfg)
					m.rebuildProfiles()
					m.clampSelected()
				} else if !p.IsNew && !p.IsNewLoop && !p.IsSeparator {
					m.globalCfg.RemoveProfile(p.Name)
					config.Save(m.globalCfg)
					m.rebuildProfiles()
					m.clampSelected()
				}
			}
			m.state = stateSelector
			return m, nil
		case "n", "esc":
			m.state = stateSelector
			return m, nil
		}
	}
	return m, nil
}

// viewConfirmDelete renders the delete confirmation prompt.
func (m AppModel) viewConfirmDelete() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRed)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorText)

	var lines []string
	lines = append(lines, sectionStyle.Render("Confirm Delete"))
	lines = append(lines, "")

	name := ""
	kind := ""
	if m.confirmDeleteIdx >= 0 && m.confirmDeleteIdx < len(m.profiles) {
		p := m.profiles[m.confirmDeleteIdx]
		if p.IsLoop {
			name = p.LoopName
			kind = "loop"
		} else {
			name = p.Name
			kind = "profile"
		}
	}

	lines = append(lines, dimStyle.Render("Are you sure you want to delete the "+kind))
	lines = append(lines, nameStyle.Render(name)+dimStyle.Render("?"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("y: yes, delete  n/esc: cancel"))

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

// startLoop transitions from selector to running a loop.
func (m AppModel) startLoop(loopName string) (tea.Model, tea.Cmd) {
	loopDef := m.globalCfg.FindLoop(loopName)
	if loopDef == nil || len(loopDef.Steps) == 0 {
		return m, nil
	}

	profiles, ok := m.profilesForLoop(loopDef.Steps)
	if !ok {
		return m, nil
	}

	return m.startLoopSession(*loopDef, profiles, loopDef.Name, "loop", nil, 0)
}

func (m AppModel) profilesForLoop(steps []config.LoopStep) ([]config.Profile, bool) {
	seen := make(map[string]struct{}, len(steps))
	profiles := make([]config.Profile, 0, len(steps))
	for _, step := range steps {
		name := strings.TrimSpace(step.Profile)
		if name == "" {
			return nil, false
		}
		if _, ok := seen[strings.ToLower(name)]; ok {
			continue
		}
		prof := m.globalCfg.FindProfile(name)
		if prof == nil {
			return nil, false
		}
		seen[strings.ToLower(name)] = struct{}{}
		profiles = append(profiles, *prof)
	}
	return profiles, true
}

func (m AppModel) startLoopSession(loopDef config.LoopDef, profiles []config.Profile, displayProfile, displayAgent string, cmdOverrides map[string]string, maxCycles int) (tea.Model, tea.Cmd) {
	if err := m.store.EnsureDirs(); err != nil {
		m.selectorMsg = "Start failed: " + err.Error()
		return m, nil
	}

	workDir := ""
	if m.project != nil {
		workDir = m.project.RepoPath
	}
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	projectName := ""
	if m.project != nil {
		projectName = m.project.Name
	}

	dcfg := session.DaemonConfig{
		ProjectDir:            workDir,
		ProjectName:           projectName,
		WorkDir:               workDir,
		PlanID:                activePlanID(m.project, m.plan),
		ProfileName:           displayProfile,
		AgentName:             displayAgent,
		Loop:                  loopDef,
		Profiles:              profiles,
		Pushover:              m.globalCfg.Pushover,
		MaxCycles:             maxCycles,
		AgentCommandOverrides: cmdOverrides,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		m.selectorMsg = "Start failed: " + err.Error()
		return m, nil
	}
	if err := session.StartDaemon(sessionID); err != nil {
		session.AbortSessionStartup(sessionID, "tui start daemon failed: "+err.Error())
		m.selectorMsg = "Start failed: " + err.Error()
		return m, nil
	}

	client, err := session.ConnectToSession(sessionID)
	if err != nil {
		session.AbortSessionStartup(sessionID, "tui attach failed: "+err.Error())
		m.selectorMsg = "Start failed: " + err.Error()
		return m, nil
	}

	eventCh := make(chan any, 256)
	cancelFunc := func() {
		_ = client.Cancel()
	}

	go func() {
		client.StreamEvents(eventCh, nil)
	}()

	m.selectorMsg = ""
	m.state = stateRunning
	m.runCancel = cancelFunc
	m.runEventCh = eventCh
	m.sessionClient = client
	m.sessionID = sessionID
	m.runModel = runtui.NewModel(projectName, m.plan, displayAgent, "", eventCh, cancelFunc)
	m.runModel.SetStore(m.store)
	m.runModel.SetSessionMode(sessionID)
	m.runModel.SetLoopInfo(loopDef.Name, len(loopDef.Steps))
	m.runModel.SetSize(m.width, m.height)

	return m, m.runModel.Init()
}

// startEditLoop pre-populates the loop wizard fields from an existing loop
// and opens the loop edit menu.
func (m AppModel) startEditLoop() (tea.Model, tea.Cmd) {
	p := m.profiles[m.selected]
	loopDef := m.globalCfg.FindLoop(p.LoopName)
	if loopDef == nil {
		return m, nil
	}

	m.loopEditing = true
	m.loopEditName = loopDef.Name
	m.loopNameInput = loopDef.Name
	m.loopSteps = make([]config.LoopStep, len(loopDef.Steps))
	copy(m.loopSteps, loopDef.Steps)
	m.loopStepSel = 0
	m.loopMenuSel = 0
	m.state = stateLoopMenu
	return m, nil
}

func (m AppModel) attachToSession(target session.SessionMeta) (tea.Model, tea.Cmd) {
	client, err := session.ConnectToSession(target.ID)
	if err != nil {
		m.selectorMsg = "Attach failed: " + err.Error()
		m.state = stateSelector
		return m, nil
	}

	eventCh := make(chan any, 256)
	cancelFunc := func() {
		_ = client.Cancel()
	}

	go func() {
		client.StreamEvents(eventCh, nil)
	}()

	m.selectorMsg = ""
	m.state = stateRunning
	m.runCancel = cancelFunc
	m.runEventCh = eventCh
	m.sessionClient = client
	m.sessionID = target.ID
	m.runModel = runtui.NewModel(target.ProjectName, m.plan, target.AgentName, "", eventCh, cancelFunc)
	m.runModel.SetStore(m.store)
	m.runModel.SetSessionMode(target.ID)
	if target.LoopName != "" {
		m.runModel.SetLoopInfo(target.LoopName, target.LoopSteps)
	}
	m.runModel.SetSize(m.width, m.height)

	return m, m.runModel.Init()
}

// showSessions opens a session picker when multiple active sessions exist.
func (m AppModel) showSessions() (tea.Model, tea.Cmd) {
	if session.IsAgentContext() {
		return m, nil
	}

	active, err := session.ListActiveSessions()
	if err != nil {
		m.selectorMsg = "Load sessions failed: " + err.Error()
		return m, nil
	}
	if len(active) == 0 {
		m.selectorMsg = "No active sessions."
		return m, nil
	}

	if len(active) == 1 {
		return m.attachToSession(active[0])
	}

	m.activeSessions = active
	if m.sessionPickSel < 0 || m.sessionPickSel >= len(active) {
		m.sessionPickSel = 0
	}
	m.state = stateSessionPicker
	return m, nil
}

func (m AppModel) updateSessionPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.state = stateSelector
			return m, nil
		case "r":
			active, err := session.ListActiveSessions()
			if err != nil {
				m.selectorMsg = "Refresh failed: " + err.Error()
				return m, nil
			}
			if len(active) == 0 {
				m.selectorMsg = "No active sessions."
				m.state = stateSelector
				return m, nil
			}
			m.activeSessions = active
			if m.sessionPickSel >= len(m.activeSessions) {
				m.sessionPickSel = len(m.activeSessions) - 1
			}
			if m.sessionPickSel < 0 {
				m.sessionPickSel = 0
			}
			return m, nil
		case "j", "down":
			if len(m.activeSessions) > 0 {
				m.sessionPickSel = (m.sessionPickSel + 1) % len(m.activeSessions)
			}
			return m, nil
		case "k", "up":
			if len(m.activeSessions) > 0 {
				m.sessionPickSel = (m.sessionPickSel - 1 + len(m.activeSessions)) % len(m.activeSessions)
			}
			return m, nil
		case "enter":
			if len(m.activeSessions) == 0 {
				m.selectorMsg = "No active sessions."
				m.state = stateSelector
				return m, nil
			}
			target := m.activeSessions[m.sessionPickSel]
			return m.attachToSession(target)
		}
	}
	return m, nil
}

func (m AppModel) updateRunning(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Intercept BackToSelectorMsg to return to selector.
	if _, ok := msg.(runtui.BackToSelectorMsg); ok {
		// Cancel any remaining agent context.
		if m.runCancel != nil {
			m.runCancel()
			m.runCancel = nil
		}
		if m.sessionClient != nil {
			m.sessionClient.Close()
			m.sessionClient = nil
		}
		m.state = stateSelector
		m.runEventCh = nil
		m.sessionID = 0
		m.loadProjectData()
		return m, tea.SetWindowTitle("adaf")
	}

	// Intercept DetachMsg to detach from the session without stopping the agent.
	if detach, ok := msg.(runtui.DetachMsg); ok {
		if m.sessionClient != nil {
			m.sessionClient.Close()
			m.sessionClient = nil
		}
		m.state = stateSelector
		m.runEventCh = nil
		m.runCancel = nil
		m.sessionID = 0
		m.loadProjectData()
		_ = detach // session continues in background
		return m, tea.SetWindowTitle("adaf")
	}

	updated, cmd := m.runModel.Update(msg)
	m.runModel = updated.(runtui.Model)
	return m, cmd
}

// startAgent transitions from selector to running state.
// Everything now runs through loop sessions (single-profile runs are a 1-step loop).
func (m AppModel) startAgent() (tea.Model, tea.Cmd) {
	p := m.profiles[m.selected]

	// Skip separators.
	if p.IsSeparator {
		return m, nil
	}

	// If it's the "new loop" sentinel, start loop creation.
	if p.IsNewLoop {
		m.loopEditing = false
		m.loopEditName = ""
		m.loopNameInput = ""
		m.loopSteps = nil
		m.state = stateLoopName
		return m, nil
	}

	// If it's the sentinel entry, start profile creation.
	if p.IsNew {
		m.profileNameInput = ""
		m.state = stateProfileName
		return m, nil
	}

	// If it's a loop, start the loop runner.
	if p.IsLoop {
		return m.startLoop(p.LoopName)
	}

	prof := m.globalCfg.FindProfile(p.Name)
	if prof == nil {
		return m, nil
	}

	loopDef := config.LoopDef{
		Name: "profile:" + prof.Name,
		Steps: []config.LoopStep{
			{Profile: prof.Name, Turns: 1},
		},
	}
	return m.startLoopSession(loopDef, []config.Profile{*prof}, prof.Name, prof.Agent, nil, 0)
}

// View implements tea.Model.
func (m AppModel) View() string {
	if m.width == 0 || m.height < 3 {
		return "Loading..."
	}

	switch m.state {
	case stateRunning:
		return m.runModel.View()
	case statePlanMenu:
		return m.viewPlanMenu()
	case statePlanCreateID:
		return m.viewPlanCreateID()
	case statePlanCreateTitle:
		return m.viewPlanCreateTitle()
	case stateProfileName:
		return m.viewProfileName()
	case stateProfileAgent:
		return m.viewProfileAgent()
	case stateProfileModel:
		return m.viewProfileModel()
	case stateProfileReasoning:
		return m.viewProfileReasoning()
	case stateProfileRole:
		return m.viewProfileRole()
	case stateProfileIntel:
		return m.viewProfileIntel()
	case stateProfileDesc:
		return m.viewProfileDesc()
	case stateProfileMaxInst:
		return m.viewProfileMaxInst()
	case stateProfileSpawnable:
		return m.viewProfileSpawnable()
	case stateProfileMaxPar:
		return m.viewProfileMaxPar()
	case stateProfileMenu:
		return m.viewProfileMenu()
	case stateLoopName:
		return m.viewLoopName()
	case stateLoopStepList:
		return m.viewLoopStepList()
	case stateLoopStepProfile:
		return m.viewLoopStepProfile()
	case stateLoopStepTurns:
		return m.viewLoopStepTurns()
	case stateLoopStepInstr:
		return m.viewLoopStepInstr()
	case stateLoopStepTools:
		return m.viewLoopStepTools()
	case stateLoopMenu:
		return m.viewLoopMenu()
	case stateSettings:
		return m.viewSettings()
	case stateSettingsPushoverUserKey:
		return m.viewSettingsPushoverUserKey()
	case stateSettingsPushoverAppToken:
		return m.viewSettingsPushoverAppToken()
	case stateSessionPicker:
		return m.viewSessionPicker()
	case stateConfirmDelete:
		return m.viewConfirmDelete()
	default:
		return m.viewSelector()
	}
}

func (m AppModel) viewSelector() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()

	panelH := m.height - 2
	if panelH < 1 {
		panelH = 1
	}

	panels := renderSelector(
		m.profiles,
		m.selected,
		m.project,
		m.plan,
		m.plans,
		m.issues,
		m.docs,
		m.logs,
		m.activeSessions,
		m.activeLoop,
		m.cachedProfileStats,
		m.cachedLoopStats,
		m.width,
		m.height,
	)
	return header + "\n" + panels + "\n" + statusBar
}

func (m AppModel) viewSessionPicker() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	var lines []string
	lines = append(lines, sectionStyle.Render("Active Sessions"))
	lines = append(lines, "")

	if len(m.activeSessions) == 0 {
		lines = append(lines, dimStyle.Render("No active sessions."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("esc/q: back"))
	} else {
		for i, s := range m.activeSessions {
			prefix := "  "
			if i == m.sessionPickSel {
				prefix = "> "
			}
			title := fmt.Sprintf("#%d %s/%s", s.ID, s.ProfileName, s.AgentName)
			status := selectorRuntimeStatusStyle(s.Status).Render("[" + s.Status + "]")
			lines = append(lines, prefix+valueStyle.Render(truncateInputForDisplay(title, cw-18))+" "+status)

			detailParts := make([]string, 0, 3)
			if s.ProjectName != "" {
				detailParts = append(detailParts, "project="+s.ProjectName)
			}
			if s.LoopName != "" {
				detailParts = append(detailParts, "loop="+s.LoopName)
			}
			detailParts = append(detailParts, "started="+s.StartedAt.Local().Format("15:04:05"))
			lines = append(lines, "  "+dimStyle.Render(strings.Join(detailParts, "  ")))
		}
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("enter: attach  j/k: select  r: refresh  esc/q: back"))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

func (m AppModel) renderHeader() string {
	title := " adaf"
	if m.project != nil && m.project.Name != "" {
		title += " — " + m.project.Name
	}
	if id := activePlanID(m.project, m.plan); id != "" {
		title += " [" + id + "]"
	}
	title += " "
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBase).
		Background(ColorBlue).
		Padding(0, 2).
		Width(m.width).
		MaxWidth(m.width).
		Render(title)
}

func (m AppModel) renderStatusBar() string {
	var parts []string
	add := func(key, desc string) {
		parts = append(parts,
			lipgloss.NewStyle().Bold(true).Foreground(ColorLavender).Background(ColorSurface0).Render(key)+
				lipgloss.NewStyle().Foreground(ColorSubtext0).Background(ColorSurface0).Render(" "+desc))
	}

	add("j/k", "navigate")
	add("enter", "start")
	if m.state == stateSelector {
		add("n", "new profile")
		add("l", "new loop")
		add("p", "plans")
		add("[/]", "cycle plan")
		add("e", "edit")
		add("d", "delete")
		add("s", "sessions")
		add("S", "settings")
		if msg := strings.TrimSpace(m.selectorMsg); msg != "" {
			maxW := m.width / 3
			if maxW < 20 {
				maxW = 20
			}
			add("!", truncateInputForDisplay(msg, maxW))
		}
	}
	if m.state == stateSessionPicker {
		add("r", "refresh")
		add("esc", "back")
	}
	add("q", "quit")

	content := strings.Join(parts, lipgloss.NewStyle().Foreground(ColorSubtext0).Background(ColorSurface0).Render("  "))
	return lipgloss.NewStyle().
		Foreground(ColorSubtext0).
		Background(ColorSurface0).
		Padding(0, 1).
		Width(m.width).
		MaxWidth(m.width).
		Render(content)
}

// RunApp launches the unified TUI application.
func RunApp(s *store.Store) error {
	m := NewApp(s)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func activePlanID(project *store.ProjectConfig, plan *store.Plan) string {
	if plan != nil && strings.TrimSpace(plan.ID) != "" {
		return strings.TrimSpace(plan.ID)
	}
	if project != nil {
		return strings.TrimSpace(project.ActivePlanID)
	}
	return ""
}
