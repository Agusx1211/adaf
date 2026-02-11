package tui

import (
	"context"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/agentmeta"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/looprun"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

// appState distinguishes the modes of the unified TUI.
type appState int

const (
	stateSelector         appState = iota
	stateRunning
	stateProfileName      // text input for new profile name
	stateProfileAgent     // pick agent for new profile
	stateProfileModel     // pick model for new profile
	stateProfileReasoning // pick reasoning level for new profile
	stateProfileRole      // pick role for new profile
	stateProfileIntel     // input intelligence rating (1-10)
	stateProfileDesc      // input description text
	stateProfileMaxInst   // input max concurrent instances
	stateProfileSpawnable // multi-select spawnable profiles
	stateProfileMaxPar    // input max parallel
	stateProfileMenu      // edit profile: field picker menu
	stateLoopName         // text input for loop name
	stateLoopStepList     // list of steps, add/edit/remove/reorder
	stateLoopStepProfile  // pick profile for a step
	stateLoopStepTurns    // input turns for a step
	stateLoopStepInstr    // input custom instructions for a step
	stateLoopStepTools    // multi-select tools (stop, message, pushover)
	stateLoopMenu         // edit loop: field picker menu
	stateSettings         // settings screen (pushover credentials, etc.)
	stateSettingsPushoverUserKey  // input pushover user key
	stateSettingsPushoverAppToken // input pushover app token
)

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

	// Profile creation/editing wizard state.
	profileEditing         bool   // true = editing existing, false = creating new
	profileEditName        string // original name of profile being edited
	profileNameInput       string
	profileAgents          []string
	profileAgentSel        int
	profileModels          []string
	profileModelSel        int
	profileCustomModel     string
	profileCustomModelMode bool
	profileSelectedModel   string
	profileReasoningLevels    []agentmeta.ReasoningLevel
	profileReasoningLevelSel  int
	profileSelectedReasoning  string
	profileRoleSel            int
	profileIntelInput         string
	profileDescInput          string
	profileMenuSel            int
	profileMaxInstInput       string
	profileSpawnableOptions   []string // profile names available for selection
	profileSpawnableSelected  map[int]bool
	profileSpawnableSel       int
	profileMaxParInput        string

	// Loop creation/editing wizard state.
	loopEditing         bool
	loopEditName        string
	loopNameInput       string
	loopSteps           []config.LoopStep
	loopStepSel         int
	loopStepEditIdx     int    // which step is being edited (-1 = adding new)
	loopStepProfileOpts []string
	loopStepProfileSel  int
	loopStepTurnsInput  string
	loopStepInstrInput  string
	loopStepCanStop     bool
	loopStepCanMsg      bool
	loopStepCanPushover bool
	loopStepToolsSel    int    // cursor position in the tools multi-select
	loopMenuSel         int

	// Settings screen state.
	settingsSel              int    // cursor in settings menu
	settingsPushoverUserKey  string // input buffer for pushover user key
	settingsPushoverAppToken string // input buffer for pushover app token

	// Cached project data for the selector.
	project *store.ProjectConfig
	plan    *store.Plan
	issues  []store.Issue
	logs    []store.SessionLog

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
	m.plan, _ = m.store.LoadPlan()
	m.issues, _ = m.store.ListIssues()
	m.logs, _ = m.store.ListLogs()
	m.loadStats()
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
	return tea.SetWindowTitle("adaf")
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
	}

	switch m.state {
	case stateSelector:
		return m.updateSelector(msg)
	case stateRunning:
		return m.updateRunning(msg)
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

// startLoop transitions from selector to running a loop.
func (m AppModel) startLoop(loopName string) (tea.Model, tea.Cmd) {
	loopDef := m.globalCfg.FindLoop(loopName)
	if loopDef == nil || len(loopDef.Steps) == 0 {
		return m, nil
	}

	m.store.EnsureDirs()

	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())
	agent.PopulateFromConfig(agentsCfg)

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

	eventCh := make(chan any, 256)
	cfg := looprun.RunConfig{
		Store:     m.store,
		GlobalCfg: m.globalCfg,
		LoopDef:   loopDef,
		Project:   m.project,
		AgentsCfg: agentsCfg,
		WorkDir:   workDir,
	}

	cancel := looprun.StartLoopRun(cfg, eventCh)

	m.state = stateRunning
	m.runCancel = cancel
	m.runEventCh = eventCh
	m.runModel = runtui.NewModel(projectName, m.plan, "", "", eventCh, cancel)
	m.runModel.SetSize(m.width, m.height)
	m.runModel.SetLoopInfo(loopDef.Name, len(loopDef.Steps))

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

// showSessions lists active sessions and attaches to the first one found, or
// shows a message if none are active.
func (m AppModel) showSessions() (tea.Model, tea.Cmd) {
	if session.IsAgentContext() {
		return m, nil
	}

	active, err := session.ListActiveSessions()
	if err != nil || len(active) == 0 {
		return m, nil
	}

	// If there's exactly one active session, attach to it directly.
	// If multiple, attach to the most recent (first in list, which is sorted by ID desc).
	target := active[0]

	client, err := session.ConnectToSession(target.ID)
	if err != nil {
		return m, nil
	}

	eventCh := make(chan any, 256)
	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc := func() {
		client.Cancel()
		cancel()
	}

	go func() {
		client.StreamEvents(eventCh, nil)
	}()

	m.state = stateRunning
	m.runCancel = cancelFunc
	m.runEventCh = eventCh
	m.sessionClient = client
	m.sessionID = target.ID
	m.runModel = runtui.NewModel(target.ProjectName, m.plan, target.AgentName, "", eventCh, cancelFunc)
	m.runModel.SetSessionMode(target.ID)
	m.runModel.SetSize(m.width, m.height)

	_ = ctx
	return m, m.runModel.Init()
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
// It launches the agent via a session daemon for detach/reattach support.
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

	// Load agent config for path lookups.
	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())

	var customCmd string
	if agentsCfg != nil {
		if rec, ok := agentsCfg.Agents[p.Agent]; ok && rec.Path != "" {
			customCmd = rec.Path
		}
	}

	// The profile model IS the override — no ResolveModelOverride needed.
	modelOverride := p.Model

	// Look up reasoning level from the saved profile.
	reasoningLevel := ""
	if prof := m.globalCfg.FindProfile(p.Name); prof != nil {
		reasoningLevel = prof.ReasoningLevel
	}

	var agentArgs []string
	agentEnv := make(map[string]string)
	switch p.Agent {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentEnv["CLAUDE_CODE_EFFORT_LEVEL"] = reasoningLevel
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentArgs = append(agentArgs, "-c", `model_reasoning_effort="`+reasoningLevel+`"`)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	}

	if customCmd == "" {
		switch p.Agent {
		case "claude", "codex", "vibe", "opencode", "generic":
		default:
			customCmd = p.Agent
		}
	}

	workDir := ""
	if m.project != nil {
		workDir = m.project.RepoPath
	}
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	// Look up the full profile for role-aware prompt building.
	var prof *config.Profile
	if found := m.globalCfg.FindProfile(p.Name); found != nil {
		prof = found
	}
	prompt, _ := buildPrompt(m.store, m.project, prof, m.globalCfg)

	projectName := ""
	if m.project != nil {
		projectName = m.project.Name
	}

	// Create a session daemon config.
	dcfg := session.DaemonConfig{
		AgentName:    p.Agent,
		AgentCommand: customCmd,
		AgentArgs:    agentArgs,
		AgentEnv:     agentEnv,
		WorkDir:      workDir,
		Prompt:       prompt,
		ProjectDir:   workDir,
		ProfileName:  p.Name,
		ProjectName:  projectName,
	}

	// Allocate a session and start the daemon.
	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		// Fallback: run inline without session support.
		return m.startAgentInline(p, projectName)
	}

	if err := session.StartDaemon(sessionID); err != nil {
		// Fallback: run inline without session support.
		return m.startAgentInline(p, projectName)
	}

	// Connect to the daemon.
	client, err := session.ConnectToSession(sessionID)
	if err != nil {
		// Fallback: run inline without session support.
		return m.startAgentInline(p, projectName)
	}

	// Set up the event channel.
	eventCh := make(chan any, 256)

	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc := func() {
		client.Cancel()
		cancel()
	}

	// Stream events from the daemon.
	go func() {
		client.StreamEvents(eventCh, nil)
	}()

	m.state = stateRunning
	m.runCancel = cancelFunc
	m.runEventCh = eventCh
	m.sessionClient = client
	m.sessionID = sessionID
	m.runModel = runtui.NewModel(projectName, m.plan, p.Agent, "", eventCh, cancelFunc)
	m.runModel.SetSessionMode(sessionID)
	m.runModel.SetSize(m.width, m.height)

	_ = ctx // cancel is wrapped in cancelFunc
	return m, m.runModel.Init()
}

// startAgentInline is the fallback: runs the agent in-process without session
// daemon support (no detach/reattach). Used when session creation fails.
func (m AppModel) startAgentInline(p profileEntry, projectName string) (tea.Model, tea.Cmd) {
	agentInstance, ok := agent.Get(p.Agent)
	if !ok {
		return m, nil
	}

	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())
	var customCmd string
	if agentsCfg != nil {
		if rec, ok := agentsCfg.Agents[p.Agent]; ok && rec.Path != "" {
			customCmd = rec.Path
		}
	}

	modelOverride := p.Model
	reasoningLevel := ""
	if prof := m.globalCfg.FindProfile(p.Name); prof != nil {
		reasoningLevel = prof.ReasoningLevel
	}

	var agentArgs []string
	agentEnv := make(map[string]string)
	switch p.Agent {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentEnv["CLAUDE_CODE_EFFORT_LEVEL"] = reasoningLevel
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		if reasoningLevel != "" {
			agentArgs = append(agentArgs, "-c", `model_reasoning_effort="`+reasoningLevel+`"`)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	}

	if customCmd == "" {
		switch p.Agent {
		case "claude", "codex", "vibe", "opencode", "generic":
		default:
			customCmd = p.Agent
		}
	}

	workDir := ""
	if m.project != nil {
		workDir = m.project.RepoPath
	}
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	var prof *config.Profile
	if found := m.globalCfg.FindProfile(p.Name); found != nil {
		prof = found
	}
	prompt, _ := buildPrompt(m.store, m.project, prof, m.globalCfg)

	agentCfg := agent.Config{
		Name:    p.Agent,
		Command: customCmd,
		Args:    agentArgs,
		Env:     agentEnv,
		WorkDir: workDir,
		Prompt:  prompt,
	}

	eventCh := make(chan any, 256)
	cancel := runtui.StartAgentLoop(runtui.RunConfig{
		Store:       m.store,
		Agent:       agentInstance,
		AgentCfg:    agentCfg,
		Plan:        m.plan,
		ProjectName: projectName,
		ProfileName: p.Name,
	}, eventCh)

	m.state = stateRunning
	m.runCancel = cancel
	m.runEventCh = eventCh
	m.runModel = runtui.NewModel(projectName, m.plan, p.Agent, "", eventCh, cancel)
	m.runModel.SetSize(m.width, m.height)

	return m, m.runModel.Init()
}

// View implements tea.Model.
func (m AppModel) View() string {
	if m.width == 0 || m.height < 3 {
		return "Loading..."
	}

	switch m.state {
	case stateRunning:
		return m.runModel.View()
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

	panels := renderSelector(m.profiles, m.selected, m.project, m.plan, m.issues, m.logs, m.cachedProfileStats, m.cachedLoopStats, m.width, m.height)
	return header + "\n" + panels + "\n" + statusBar
}

func (m AppModel) renderHeader() string {
	title := " adaf"
	if m.project != nil && m.project.Name != "" {
		title += " — " + m.project.Name
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
		add("e", "edit")
		add("d", "delete")
		add("s", "sessions")
		add("S", "settings")
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

// buildPrompt constructs a default prompt from project context using the shared builder.
func buildPrompt(s *store.Store, project *store.ProjectConfig, profile *config.Profile, globalCfg *config.GlobalConfig) (string, error) {
	return promptpkg.Build(promptpkg.BuildOpts{
		Store:     s,
		Project:   project,
		Profile:   profile,
		GlobalCfg: globalCfg,
	})
}
