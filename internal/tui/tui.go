package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/store"
)

// View represents the currently active view.
type View int

const (
	ViewDashboard View = iota
	ViewPlan
	ViewIssues
	ViewLogs
	ViewSessions
	ViewDocs
)

var viewNames = []string{"Dashboard", "Plan", "Issues", "Logs", "Sessions", "Docs"}

// Model is the top-level bubbletea model for the ADAF TUI.
type Model struct {
	store  *store.Store
	keys   ViewKeyMap
	width  int
	height int

	// Current view
	activeView View

	// Data loaded from store
	project    store.ProjectConfig
	plan       *store.Plan
	issues     []store.Issue
	logs       []store.SessionLog
	recordings []store.SessionRecording
	docs       []store.Doc

	// View-specific state
	planSelectedIdx int
	planShowDetail  bool

	issueSelectedIdx int
	issueShowDetail  bool

	logSelectedIdx int
	logShowDetail  bool

	sessionSelectedIdx int
	sessionShowDetail  bool

	docSelectedIdx int
	docShowDetail  bool

	// Error state
	err error
}

// New creates a new TUI model and loads data from the given store.
func New(s *store.Store) Model {
	m := Model{
		store: s,
		keys:  DefaultKeyMap(),
	}
	m.loadData()
	return m
}

// loadData reads all data from the store into the model.
func (m *Model) loadData() {
	// Load project config
	proj, err := m.store.LoadProject()
	if err != nil {
		m.err = fmt.Errorf("loading project: %w", err)
	} else if proj != nil {
		m.project = *proj
	}

	// Load plan
	plan, err := m.store.LoadPlan()
	if err != nil {
		m.err = fmt.Errorf("loading plan: %w", err)
	} else {
		m.plan = plan
	}

	// Load issues
	issues, err := m.store.ListIssues()
	if err != nil {
		m.err = fmt.Errorf("loading issues: %w", err)
	} else {
		m.issues = issues
	}

	// Load session logs
	logs, err := m.store.ListLogs()
	if err != nil {
		m.err = fmt.Errorf("loading logs: %w", err)
	} else {
		m.logs = logs
	}

	// Load docs
	docs, err := m.store.ListDocs()
	if err != nil {
		m.err = fmt.Errorf("loading docs: %w", err)
	} else {
		m.docs = docs
	}

	// Load recordings - enumerate the recordings directory
	m.recordings = m.loadRecordings()
}

// loadRecordings scans both records/ and legacy recordings/ directories.
func (m *Model) loadRecordings() []store.SessionRecording {
	seen := make(map[int]bool)
	var recordings []store.SessionRecording

	for _, dir := range m.store.RecordsDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			sessionID, err := strconv.Atoi(entry.Name())
			if err != nil || seen[sessionID] {
				continue
			}
			rec, err := m.store.LoadRecording(sessionID)
			if err != nil {
				continue
			}
			seen[sessionID] = true
			recordings = append(recordings, *rec)
		}
	}
	return recordings
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.SetWindowTitle("ADAF - Autonomous Developer Agent Flow")
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global key handling

		// Quit
		if key.Matches(msg, m.keys.Quit) {
			// Only quit if not in a detail view
			if m.inDetailView() {
				m.closeDetailView()
				return m, nil
			}
			return m, tea.Quit
		}

		// If in a detail view, handle escape to go back
		if key.Matches(msg, m.keys.Escape) {
			if m.inDetailView() {
				m.closeDetailView()
				return m, nil
			}
		}

		// Tab navigation (not in detail view)
		if !m.inDetailView() {
			if key.Matches(msg, m.keys.Tab) {
				m.activeView = (m.activeView + 1) % 6
				return m, nil
			}
			if key.Matches(msg, m.keys.ShiftTab) {
				m.activeView = (m.activeView + 5) % 6
				return m, nil
			}

			// Number key navigation
			if key.Matches(msg, m.keys.View1) {
				m.activeView = ViewDashboard
				return m, nil
			}
			if key.Matches(msg, m.keys.View2) {
				m.activeView = ViewPlan
				return m, nil
			}
			if key.Matches(msg, m.keys.View3) {
				m.activeView = ViewIssues
				return m, nil
			}
			if key.Matches(msg, m.keys.View4) {
				m.activeView = ViewLogs
				return m, nil
			}
			if key.Matches(msg, m.keys.View5) {
				m.activeView = ViewSessions
				return m, nil
			}
			if key.Matches(msg, m.keys.View6) {
				m.activeView = ViewDocs
				return m, nil
			}
		}

		// View-specific key handling
		return m.handleViewKeys(msg)
	}

	return m, nil
}

// handleViewKeys dispatches key events to the active view's handler.
func (m Model) handleViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.activeView {
	case ViewPlan:
		return m.handlePlanKeys(msg)
	case ViewIssues:
		return m.handleIssuesKeys(msg)
	case ViewLogs:
		return m.handleLogsKeys(msg)
	case ViewSessions:
		return m.handleSessionsKeys(msg)
	case ViewDocs:
		return m.handleDocsKeys(msg)
	}
	return m, nil
}

func (m Model) handlePlanKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.plan == nil || len(m.plan.Phases) == 0 {
		return m, nil
	}
	if key.Matches(msg, m.keys.Up) {
		if m.planSelectedIdx > 0 {
			m.planSelectedIdx--
		}
	}
	if key.Matches(msg, m.keys.Down) {
		if m.planSelectedIdx < len(m.plan.Phases)-1 {
			m.planSelectedIdx++
		}
	}
	if key.Matches(msg, m.keys.Enter) {
		m.planShowDetail = !m.planShowDetail
	}
	if key.Matches(msg, m.keys.Escape) {
		m.planShowDetail = false
	}
	return m, nil
}

func (m Model) handleIssuesKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.issues) == 0 {
		return m, nil
	}
	if key.Matches(msg, m.keys.Up) {
		if m.issueSelectedIdx > 0 {
			m.issueSelectedIdx--
		}
	}
	if key.Matches(msg, m.keys.Down) {
		if m.issueSelectedIdx < len(m.issues)-1 {
			m.issueSelectedIdx++
		}
	}
	if key.Matches(msg, m.keys.Enter) {
		m.issueShowDetail = !m.issueShowDetail
	}
	if key.Matches(msg, m.keys.Escape) {
		m.issueShowDetail = false
	}
	return m, nil
}

func (m Model) handleLogsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.logs) == 0 {
		return m, nil
	}
	if key.Matches(msg, m.keys.Up) {
		if m.logSelectedIdx > 0 {
			m.logSelectedIdx--
		}
	}
	if key.Matches(msg, m.keys.Down) {
		if m.logSelectedIdx < len(m.logs)-1 {
			m.logSelectedIdx++
		}
	}
	if key.Matches(msg, m.keys.Enter) {
		m.logShowDetail = !m.logShowDetail
	}
	if key.Matches(msg, m.keys.Escape) {
		m.logShowDetail = false
	}
	return m, nil
}

func (m Model) handleSessionsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.recordings) == 0 {
		return m, nil
	}
	if key.Matches(msg, m.keys.Up) {
		if m.sessionSelectedIdx > 0 {
			m.sessionSelectedIdx--
		}
	}
	if key.Matches(msg, m.keys.Down) {
		if m.sessionSelectedIdx < len(m.recordings)-1 {
			m.sessionSelectedIdx++
		}
	}
	if key.Matches(msg, m.keys.Enter) {
		m.sessionShowDetail = !m.sessionShowDetail
	}
	if key.Matches(msg, m.keys.Escape) {
		m.sessionShowDetail = false
	}
	return m, nil
}

func (m Model) handleDocsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.docs) == 0 {
		return m, nil
	}
	if key.Matches(msg, m.keys.Up) {
		if m.docSelectedIdx > 0 {
			m.docSelectedIdx--
		}
	}
	if key.Matches(msg, m.keys.Down) {
		if m.docSelectedIdx < len(m.docs)-1 {
			m.docSelectedIdx++
		}
	}
	if key.Matches(msg, m.keys.Enter) {
		m.docShowDetail = !m.docShowDetail
	}
	if key.Matches(msg, m.keys.Escape) {
		m.docShowDetail = false
	}
	return m, nil
}

// inDetailView returns true if any view is showing a detail panel.
func (m Model) inDetailView() bool {
	switch m.activeView {
	case ViewPlan:
		return m.planShowDetail
	case ViewIssues:
		return m.issueShowDetail
	case ViewLogs:
		return m.logShowDetail
	case ViewSessions:
		return m.sessionShowDetail
	case ViewDocs:
		return m.docShowDetail
	}
	return false
}

// closeDetailView closes the active detail view.
func (m *Model) closeDetailView() {
	switch m.activeView {
	case ViewPlan:
		m.planShowDetail = false
	case ViewIssues:
		m.issueShowDetail = false
	case ViewLogs:
		m.logShowDetail = false
	case ViewSessions:
		m.sessionShowDetail = false
	case ViewDocs:
		m.docShowDetail = false
	}
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	sections = append(sections, m.renderHeader())

	// Tab bar
	sections = append(sections, m.renderTabBar())

	// Error message if any
	if m.err != nil {
		sections = append(sections, ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	// Main content
	content := m.renderContent()
	sections = append(sections, content)

	// Join everything
	main := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Calculate remaining space for the status bar
	mainHeight := lipgloss.Height(main)
	statusBar := m.renderStatusBar()
	statusHeight := lipgloss.Height(statusBar)

	// Fill empty space between content and status bar
	emptySpace := m.height - mainHeight - statusHeight
	if emptySpace > 0 {
		main += strings.Repeat("\n", emptySpace)
	}

	return main + "\n" + statusBar
}

// renderHeader renders the top header bar.
func (m Model) renderHeader() string {
	projectName := m.project.Name
	if projectName == "" {
		projectName = "ADAF Project"
	}

	leftContent := fmt.Sprintf(" ADAF  %s", projectName)
	rightContent := ""
	if m.project.RepoPath != "" {
		rightContent = fmt.Sprintf("%s ", m.project.RepoPath)
	}

	leftPart := HeaderStyle.Render(leftContent)
	rightPart := lipgloss.NewStyle().
		Foreground(ColorSurface1).
		Background(ColorBlue).
		Padding(0, 2).
		Render(rightContent)

	// Fill the gap between left and right
	leftWidth := lipgloss.Width(leftPart)
	rightWidth := lipgloss.Width(rightPart)
	gapWidth := m.width - leftWidth - rightWidth
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := lipgloss.NewStyle().
		Background(ColorBlue).
		Render(strings.Repeat(" ", gapWidth))

	return leftPart + gap + rightPart
}

// renderTabBar renders the navigation tab bar.
func (m Model) renderTabBar() string {
	var tabs []string
	for i, name := range viewNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if View(i) == m.activeView {
			tabs = append(tabs, ActiveTabStyle.Render(label))
		} else {
			tabs = append(tabs, InactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return TabBarStyle.Render(tabBar)
}

// renderContent renders the main content area based on the active view.
func (m Model) renderContent() string {
	padding := lipgloss.NewStyle().Padding(0, 1)
	var content string
	switch m.activeView {
	case ViewDashboard:
		content = m.dashboardView()
	case ViewPlan:
		content = m.planView()
	case ViewIssues:
		content = m.issuesView()
	case ViewLogs:
		content = m.logsView()
	case ViewSessions:
		content = m.sessionsView()
	case ViewDocs:
		content = m.docsView()
	default:
		content = EmptyStateStyle.Render("Unknown view")
	}
	return padding.Render(content)
}

// renderStatusBar renders the bottom status bar with keyboard shortcuts.
func (m Model) renderStatusBar() string {
	// Left side: current view name
	viewLabel := StatusKeyStyle.Render(fmt.Sprintf(" %s ", viewNames[m.activeView]))

	// Right side: contextual keyboard shortcuts
	var shortcuts []string
	if m.inDetailView() {
		shortcuts = append(shortcuts,
			shortcutText("esc", "back"),
		)
	} else {
		shortcuts = append(shortcuts,
			shortcutText("tab", "next view"),
			shortcutText("1-6", "jump"),
		)
	}

	switch m.activeView {
	case ViewPlan, ViewIssues, ViewLogs, ViewSessions, ViewDocs:
		if !m.inDetailView() {
			shortcuts = append(shortcuts,
				shortcutText("j/k", "navigate"),
				shortcutText("enter", "details"),
			)
		}
	}

	shortcuts = append(shortcuts, shortcutText("q", "quit"))

	rightContent := strings.Join(shortcuts, StatusValueStyle.Render("  "))

	// Build the full status bar
	leftWidth := lipgloss.Width(viewLabel)
	rightWidth := lipgloss.Width(rightContent)
	gapWidth := m.width - leftWidth - rightWidth - 2
	if gapWidth < 0 {
		gapWidth = 0
	}
	gap := StatusBarStyle.Render(strings.Repeat(" ", gapWidth))

	return StatusBarStyle.Width(m.width).Render(viewLabel + gap + rightContent)
}

// shortcutText renders a key shortcut with styling.
func shortcutText(keyStr, desc string) string {
	return StatusKeyStyle.Render(keyStr) + StatusValueStyle.Render(" "+desc)
}

// Run starts the TUI application.
func Run(s *store.Store) error {
	m := New(s)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
