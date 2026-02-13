package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/store"
)

var tuiPlanIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (m AppModel) openPlanManager() (tea.Model, tea.Cmd) {
	m.loadProjectData()
	m.planActionMsg = ""
	m.planSel = 0
	activeID := activePlanID(m.project, m.plan)
	for i, p := range m.plans {
		if p.ID == activeID {
			m.planSel = i
			break
		}
	}
	m.state = statePlanMenu
	return m, nil
}

func (m AppModel) updatePlanMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			m.state = stateSelector
			return m, nil
		case "j", "down":
			if len(m.plans) > 0 {
				m.planSel = (m.planSel + 1) % len(m.plans)
			}
			return m, nil
		case "k", "up":
			if len(m.plans) > 0 {
				m.planSel = (m.planSel - 1 + len(m.plans)) % len(m.plans)
			}
			return m, nil
		case "enter":
			p := m.selectedPlan()
			if p == nil {
				return m, nil
			}
			status := p.Status
			if status == "" {
				status = "active"
			}
			if status != "active" {
				m.planActionMsg = fmt.Sprintf("Cannot switch to %q plan", status)
				return m, nil
			}
			if err := m.store.SetActivePlan(p.ID); err != nil {
				m.planActionMsg = "Switch failed: " + err.Error()
				return m, nil
			}
			m.loadProjectData()
			m.planActionMsg = "Active plan set to " + p.ID
			return m, nil
		case "n":
			if err := m.store.SetActivePlan(""); err != nil {
				m.planActionMsg = "Clear failed: " + err.Error()
				return m, nil
			}
			m.loadProjectData()
			m.planActionMsg = "Active plan cleared"
			return m, nil
		case "c":
			m.planCreateIDInput = ""
			m.planCreateTitleInput = ""
			m.state = statePlanCreateID
			return m, nil
		case "f":
			return m.applySelectedPlanStatus("frozen")
		case "u":
			return m.applySelectedPlanStatus("active")
		case "a":
			return m.applySelectedPlanStatus("done")
		case "x":
			return m.applySelectedPlanStatus("cancelled")
		case "d":
			p := m.selectedPlan()
			if p == nil {
				return m, nil
			}
			if p.Status != "done" && p.Status != "cancelled" {
				m.planActionMsg = "Only done/cancelled plans can be deleted"
				return m, nil
			}
			if err := m.store.DeletePlan(p.ID); err != nil {
				m.planActionMsg = "Delete failed: " + err.Error()
				return m, nil
			}
			m.loadProjectData()
			if m.planSel >= len(m.plans) && m.planSel > 0 {
				m.planSel--
			}
			m.planActionMsg = "Deleted plan " + p.ID
			return m, nil
		case "r":
			m.loadProjectData()
			m.planActionMsg = "Refreshed"
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) updatePlanCreateID(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			id := strings.TrimSpace(m.planCreateIDInput)
			if id == "" {
				return m, nil
			}
			if !tuiPlanIDPattern.MatchString(id) {
				m.planActionMsg = "Invalid ID format"
				return m, nil
			}
			if existing, _ := m.store.GetPlan(id); existing != nil {
				m.planActionMsg = "Plan already exists"
				return m, nil
			}
			m.planCreateIDInput = id
			m.planCreateTitleInput = ""
			m.state = statePlanCreateTitle
			return m, nil
		case "esc":
			m.state = statePlanMenu
			return m, nil
		case "backspace":
			if len(m.planCreateIDInput) > 0 {
				m.planCreateIDInput = m.planCreateIDInput[:len(m.planCreateIDInput)-1]
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.planCreateIDInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) updatePlanCreateTitle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			title := strings.TrimSpace(m.planCreateTitleInput)
			if title == "" {
				return m, nil
			}
			plan := &store.Plan{
				ID:     m.planCreateIDInput,
				Title:  title,
				Status: "active",
			}
			if err := m.store.CreatePlan(plan); err != nil {
				m.planActionMsg = "Create failed: " + err.Error()
				return m, nil
			}
			if m.project != nil && strings.TrimSpace(m.project.ActivePlanID) == "" {
				_ = m.store.SetActivePlan(plan.ID)
			}
			m.loadProjectData()
			m.planActionMsg = "Created plan " + plan.ID
			m.planCreateIDInput = ""
			m.planCreateTitleInput = ""
			m.state = statePlanMenu
			for i, p := range m.plans {
				if p.ID == plan.ID {
					m.planSel = i
					break
				}
			}
			return m, nil
		case "esc":
			m.state = statePlanCreateID
			return m, nil
		case "backspace":
			if len(m.planCreateTitleInput) > 0 {
				m.planCreateTitleInput = m.planCreateTitleInput[:len(m.planCreateTitleInput)-1]
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.planCreateTitleInput += msg.String()
			}
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) selectedPlan() *store.Plan {
	if m.planSel < 0 || m.planSel >= len(m.plans) {
		return nil
	}
	p := m.plans[m.planSel]
	return &p
}

func (m AppModel) applySelectedPlanStatus(status string) (tea.Model, tea.Cmd) {
	p := m.selectedPlan()
	if p == nil {
		return m, nil
	}
	msg, err := m.applyPlanStatus(p.ID, status)
	if err != nil {
		m.planActionMsg = err.Error()
		return m, nil
	}
	m.planActionMsg = msg
	return m, nil
}

func (m AppModel) applyPlanStatus(planID, newStatus string) (string, error) {
	plan, err := m.store.GetPlan(planID)
	if err != nil {
		return "", err
	}
	if plan == nil {
		return "", fmt.Errorf("plan %q not found", planID)
	}
	oldStatus := plan.Status
	if oldStatus == "" {
		oldStatus = "active"
	}
	if oldStatus == newStatus {
		return fmt.Sprintf("Plan %s already %s", planID, newStatus), nil
	}

	plan.Status = newStatus
	if err := m.store.UpdatePlan(plan); err != nil {
		return "", err
	}

	mergedIssues := 0
	mergedDocs := 0
	closedIssues := 0
	switch newStatus {
	case "done":
		mergedIssues, mergedDocs, err = m.mergePlanToShared(planID)
		if err != nil {
			return "", err
		}
	case "cancelled":
		closedIssues, err = m.closePlanIssuesAsWontfix(planID)
		if err != nil {
			return "", err
		}
	}

	if m.project != nil && strings.TrimSpace(m.project.ActivePlanID) == planID && newStatus != "active" {
		_ = m.store.SetActivePlan("")
	}

	m.loadProjectData()
	for i, p := range m.plans {
		if p.ID == planID {
			m.planSel = i
			break
		}
	}

	msg := fmt.Sprintf("Plan %s: %s -> %s", planID, oldStatus, newStatus)
	if newStatus == "done" {
		msg += fmt.Sprintf(" (merged %d issues, %d docs)", mergedIssues, mergedDocs)
	}
	if newStatus == "cancelled" {
		msg += fmt.Sprintf(" (closed %d issues)", closedIssues)
	}
	return msg, nil
}

func (m AppModel) mergePlanToShared(planID string) (int, int, error) {
	issues, err := m.store.ListIssues()
	if err != nil {
		return 0, 0, err
	}
	mergedIssues := 0
	for i := range issues {
		iss := issues[i]
		if iss.PlanID != planID {
			continue
		}
		if iss.Status != "open" && iss.Status != "in_progress" {
			continue
		}
		iss.PlanID = ""
		if err := m.store.UpdateIssue(&iss); err != nil {
			return 0, 0, err
		}
		mergedIssues++
	}

	docs, err := m.store.ListDocs()
	if err != nil {
		return 0, 0, err
	}
	mergedDocs := 0
	for i := range docs {
		doc := docs[i]
		if doc.PlanID != planID {
			continue
		}
		doc.PlanID = ""
		if err := m.store.UpdateDoc(&doc); err != nil {
			return 0, 0, err
		}
		mergedDocs++
	}
	return mergedIssues, mergedDocs, nil
}

func (m AppModel) closePlanIssuesAsWontfix(planID string) (int, error) {
	issues, err := m.store.ListIssues()
	if err != nil {
		return 0, err
	}
	closed := 0
	for i := range issues {
		iss := issues[i]
		if iss.PlanID != planID {
			continue
		}
		if iss.Status != "open" && iss.Status != "in_progress" {
			continue
		}
		iss.Status = "wontfix"
		if err := m.store.UpdateIssue(&iss); err != nil {
			return 0, err
		}
		closed++
	}
	return closed, nil
}

func (m AppModel) cycleActivePlan(delta int) error {
	if delta == 0 {
		return nil
	}
	var activePlans []string
	for _, p := range m.plans {
		status := p.Status
		if status == "" {
			status = "active"
		}
		if status == "active" {
			activePlans = append(activePlans, p.ID)
		}
	}
	if len(activePlans) == 0 {
		return nil
	}
	current := activePlanID(m.project, m.plan)
	idx := 0
	for i, id := range activePlans {
		if id == current {
			idx = i
			break
		}
	}
	next := (idx + delta + len(activePlans)) % len(activePlans)
	return m.store.SetActivePlan(activePlans[next])
}

func (m AppModel) viewPlanMenu() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	activeStyle := lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)

	var lines []string
	cursorLine := -1
	lines = append(lines, sectionStyle.Render("Plan Manager"))
	lines = append(lines, "")

	activeID := activePlanID(m.project, m.plan)
	if activeID == "" {
		lines = append(lines, dimStyle.Render("Active plan: (none)"))
	} else {
		lines = append(lines, activeStyle.Render("Active plan: "+activeID))
	}
	lines = append(lines, "")

	if len(m.plans) == 0 {
		lines = append(lines, dimStyle.Render("No plans yet. Press c to create one."))
	} else {
		for i, p := range m.plans {
			status := p.Status
			if status == "" {
				status = "active"
			}
			prefix := "  "
			if i == m.planSel {
				prefix = "> "
			}
			id := p.ID
			if p.ID == activeID {
				id = "● " + id
			}
			title := p.Title
			if title == "" {
				title = "(untitled)"
			}
			line := fmt.Sprintf("%s%-18s [%s] %s", prefix, id, status, title)
			if i == m.planSel {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorMauve).Bold(true).Render(truncateInputForDisplay(line, cw)))
				cursorLine = len(lines) - 1
			} else {
				lines = append(lines, lipgloss.NewStyle().Foreground(ColorText).Render(truncateInputForDisplay(line, cw)))
			}
		}
	}

	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter:switch  c:create  n:none  f:freeze  u:unfreeze"))
	lines = append(lines, dimStyle.Render("a:archive(done)  x:cancel  d:delete  r:refresh  esc:back"))
	if m.planActionMsg != "" {
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Last: "+truncateInputForDisplay(m.planActionMsg, cw-6)))
	}

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

func (m AppModel) viewPlanCreateID() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")

	var lines []string
	lines = append(lines, sectionStyle.Render("New Plan — ID"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter plan ID (lowercase, digits, -, _; max 64):"))
	lines = append(lines, "")
	lines = append(lines, "> "+lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(truncateInputForDisplay(m.planCreateIDInput, cw-4))+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: next  esc: cancel"))
	if m.planActionMsg != "" {
		lines = append(lines, dimStyle.Render("Last: "+truncateInputForDisplay(m.planActionMsg, cw-6)))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}

func (m AppModel) viewPlanCreateTitle() string {
	header := m.renderHeader()
	statusBar := m.renderStatusBar()
	style, cw, ch := profileWizardPanel(m)

	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)
	cursor := lipgloss.NewStyle().Foreground(ColorMauve).Render("_")

	var lines []string
	lines = append(lines, sectionStyle.Render("New Plan — Title"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Plan ID: "+m.planCreateIDInput))
	lines = append(lines, dimStyle.Render("Enter a title:"))
	lines = append(lines, "")
	lines = append(lines, "> "+lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(truncateInputForDisplay(m.planCreateTitleInput, cw-4))+cursor)
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: create  esc: back"))
	if m.planActionMsg != "" {
		lines = append(lines, dimStyle.Render("Last: "+truncateInputForDisplay(m.planActionMsg, cw-6)))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
