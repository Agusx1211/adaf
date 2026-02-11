package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/store"
)

// appState distinguishes the two modes of the unified TUI.
type appState int

const (
	stateSelector appState = iota
	stateRunning
)

// AppModel is the top-level bubbletea model for the unified adaf TUI.
type AppModel struct {
	store *store.Store
	state appState

	width  int
	height int

	// Selector state.
	agents   []agentEntry
	selected int

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
	m := AppModel{
		store: s,
		state: stateSelector,
	}
	m.loadProjectData()
	m.loadAgents()
	return m
}

func (m *AppModel) loadProjectData() {
	m.project, _ = m.store.LoadProject()
	m.plan, _ = m.store.LoadPlan()
	m.issues, _ = m.store.ListIssues()
	m.logs, _ = m.store.ListLogs()
}

func (m *AppModel) loadAgents() {
	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())
	agent.PopulateFromConfig(agentsCfg)
	m.agents = buildAgentList(agentsCfg)
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
			if len(m.agents) > 0 {
				m.selected = (m.selected + 1) % len(m.agents)
			}
		case "k", "up":
			if len(m.agents) > 0 {
				m.selected = (m.selected - 1 + len(m.agents)) % len(m.agents)
			}
		case "enter":
			if len(m.agents) > 0 {
				return m.startAgent()
			}
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
	a := m.agents[m.selected]

	agentInstance, ok := agent.Get(a.Name)
	if !ok {
		return m, nil
	}

	// Resolve agent config from cache.
	globalCfg, _ := config.Load()
	agentsCfg, _ := agent.LoadAgentsConfig(m.store.Root())

	var customCmd string
	if agentsCfg != nil {
		if rec, ok := agentsCfg.Agents[a.Name]; ok && rec.Path != "" {
			customCmd = rec.Path
		}
	}

	modelOverride := agent.ResolveModelOverride(agentsCfg, globalCfg, a.Name)

	var agentArgs []string
	switch a.Name {
	case "claude":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "--dangerously-skip-permissions")
	case "codex":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
		agentArgs = append(agentArgs, "--full-auto")
	case "opencode":
		if modelOverride != "" {
			agentArgs = append(agentArgs, "--model", modelOverride)
		}
	}

	if customCmd == "" {
		switch a.Name {
		case "claude", "codex", "vibe", "opencode", "generic":
		default:
			customCmd = a.Name
		}
	}

	workDir := ""
	if m.project != nil {
		workDir = m.project.RepoPath
	}
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	prompt, _ := buildPrompt(m.store, m.project)

	agentCfg := agent.Config{
		Name:    a.Name,
		Command: customCmd,
		Args:    agentArgs,
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
	m.runModel = runtui.NewModel(projectName, m.plan, a.Name, "", eventCh, cancel)
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

	panels := renderSelector(m.agents, m.selected, m.project, m.plan, m.issues, m.logs, m.width, m.height)
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

// buildPrompt constructs a default prompt from project context.
// This mirrors the logic from cli/run.go's buildDefaultPrompt.
func buildPrompt(s *store.Store, project *store.ProjectConfig) (string, error) {
	if project == nil {
		return "Explore the codebase and address any open issues.", nil
	}

	var b strings.Builder

	plan, _ := s.LoadPlan()
	latest, _ := s.LatestLog()

	b.WriteString("# Objective\n\n")
	b.WriteString("Project: " + project.Name + "\n\n")

	var currentPhase *store.PlanPhase
	if plan != nil && len(plan.Phases) > 0 {
		for i := range plan.Phases {
			p := &plan.Phases[i]
			if p.Status == "not_started" || p.Status == "in_progress" {
				currentPhase = p
				break
			}
		}
	}

	if currentPhase != nil {
		fmt.Fprintf(&b, "Your task is to work on phase **%s: %s**.\n\n", currentPhase.ID, currentPhase.Title)
		if currentPhase.Description != "" {
			b.WriteString(currentPhase.Description + "\n\n")
		}
	} else if plan != nil && plan.Title != "" {
		b.WriteString("All planned phases are complete. Look for remaining open issues or improvements.\n\n")
	} else {
		b.WriteString("No plan is set. Explore the codebase and address any open issues.\n\n")
	}

	b.WriteString("# Rules\n\n")
	b.WriteString("- Write code, run tests, and ensure everything compiles before finishing.\n")
	b.WriteString("- Focus on one coherent unit of work. Stop when the current phase (or a meaningful increment of it) is complete.\n")
	b.WriteString("- Do NOT read or write files inside the `.adaf/` directory directly. " +
		"Use `adaf` CLI commands instead (`adaf issues`, `adaf log`, `adaf plan`, etc.). " +
		"The `.adaf/` directory structure may change and direct access will be restricted in the future.\n")
	b.WriteString("\n")

	b.WriteString("# Context\n\n")

	if latest != nil {
		b.WriteString("## Last Session\n")
		if latest.Objective != "" {
			fmt.Fprintf(&b, "- Objective: %s\n", latest.Objective)
		}
		if latest.WhatWasBuilt != "" {
			fmt.Fprintf(&b, "- Built: %s\n", latest.WhatWasBuilt)
		}
		if latest.NextSteps != "" {
			fmt.Fprintf(&b, "- Next steps: %s\n", latest.NextSteps)
		}
		if latest.KnownIssues != "" {
			fmt.Fprintf(&b, "- Known issues: %s\n", latest.KnownIssues)
		}
		b.WriteString("\n")
	}

	issues, _ := s.ListIssues()
	var relevant []store.Issue
	for _, iss := range issues {
		if iss.Status == "open" || iss.Status == "in_progress" {
			relevant = append(relevant, iss)
		}
	}
	if len(relevant) > 0 {
		b.WriteString("## Open Issues\n")
		for _, iss := range relevant {
			fmt.Fprintf(&b, "- #%d [%s] %s: %s\n", iss.ID, iss.Priority, iss.Title, iss.Description)
		}
		b.WriteString("\n")
	}

	if currentPhase != nil && plan != nil && len(plan.Phases) > 1 {
		b.WriteString("## Neighboring Phases\n")
		for i, p := range plan.Phases {
			if p.ID == currentPhase.ID {
				if i > 0 {
					prev := plan.Phases[i-1]
					fmt.Fprintf(&b, "- Previous: [%s] %s: %s\n", prev.Status, prev.ID, prev.Title)
				}
				fmt.Fprintf(&b, "- **Current: [%s] %s: %s**\n", p.Status, p.ID, p.Title)
				if i < len(plan.Phases)-1 {
					next := plan.Phases[i+1]
					fmt.Fprintf(&b, "- Next: [%s] %s: %s\n", next.Status, next.ID, next.Title)
				}
				break
			}
		}
		b.WriteString("\n")
	}

	workDir := project.RepoPath
	if workDir != "" {
		agentsMD := filepath.Join(workDir, "AGENTS.md")
		if info, err := os.Stat(agentsMD); err == nil {
			const maxSize = 16 * 1024
			if info.Size() <= maxSize {
				if data, err := os.ReadFile(agentsMD); err == nil {
					b.WriteString("# AGENTS.md\n\n")
					b.WriteString("The repository includes an AGENTS.md with instructions for AI agents. Follow these:\n\n")
					b.WriteString(string(data))
					b.WriteString("\n\n")
				}
			} else {
				b.WriteString("# AGENTS.md\n\n")
				fmt.Fprintf(&b, "The repository includes an AGENTS.md file at `%s`. Read it before starting work — it contains important instructions for AI agents.\n\n", agentsMD)
			}
		}
	}

	return b.String(), nil
}
