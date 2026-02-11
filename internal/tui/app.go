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
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/runtui"
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

	// Cached project data for the selector.
	project *store.ProjectConfig
	plan    *store.Plan
	issues  []store.Issue
	logs    []store.SessionLog

	// Running state — embedded run TUI model.
	runModel   runtui.Model
	runCancel  context.CancelFunc
	runEventCh chan any
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
			}
		case "k", "up":
			if len(m.profiles) > 0 {
				m.selected = (m.selected - 1 + len(m.profiles)) % len(m.profiles)
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
		case "e":
			// Edit selected profile.
			if m.selected >= 0 && m.selected < len(m.profiles) && !m.profiles[m.selected].IsNew {
				return m.startEditProfile()
			}
			return m, nil
		case "d":
			// Delete selected profile (unless it's the sentinel).
			if m.selected >= 0 && m.selected < len(m.profiles) && !m.profiles[m.selected].IsNew {
				name := m.profiles[m.selected].Name
				m.globalCfg.RemoveProfile(name)
				config.Save(m.globalCfg)
				m.rebuildProfiles()
				if m.selected >= len(m.profiles) {
					m.selected = len(m.profiles) - 1
				}
				if m.selected < 0 {
					m.selected = 0
				}
			}
			return m, nil
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
		m.state = stateSelector
		m.runEventCh = nil
		m.loadProjectData()
		return m, tea.SetWindowTitle("adaf")
	}

	updated, cmd := m.runModel.Update(msg)
	m.runModel = updated.(runtui.Model)
	return m, cmd
}

// startAgent transitions from selector to running state.
func (m AppModel) startAgent() (tea.Model, tea.Cmd) {
	p := m.profiles[m.selected]

	// If it's the sentinel entry, start profile creation.
	if p.IsNew {
		m.profileNameInput = ""
		m.state = stateProfileName
		return m, nil
	}

	agentInstance, ok := agent.Get(p.Agent)
	if !ok {
		return m, nil
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

	agentCfg := agent.Config{
		Name:    p.Agent,
		Command: customCmd,
		Args:    agentArgs,
		Env:     agentEnv,
		WorkDir: workDir,
		Prompt:  prompt,
	}

	projectName := ""
	if m.project != nil {
		projectName = m.project.Name
	}

	eventCh := make(chan any, 256)
	cancel := runtui.StartAgentLoop(runtui.RunConfig{
		Store:       m.store,
		Agent:       agentInstance,
		AgentCfg:    agentCfg,
		Plan:        m.plan,
		ProjectName: projectName,
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

	panels := renderSelector(m.profiles, m.selected, m.project, m.plan, m.issues, m.logs, m.width, m.height)
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
		add("e", "edit")
		add("d", "delete")
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
