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

type PlanWizardState struct {
	CreateIDInput    string
	CreateTitleInput string
	ActionMsg        string
}

func (m AppModel) openPlanManager() (tea.Model, tea.Cmd) {
	m.loadProjectData()
	m.planWiz.ActionMsg = ""
	m.selector.PlanSel = 0
	activeID := activePlanID(m.project, m.plan)
	for i, p := range m.selector.Plans {
		if p.ID == activeID {
			m.selector.PlanSel = i
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
			if len(m.selector.Plans) > 0 {
				m.selector.PlanSel = (m.selector.PlanSel + 1) % len(m.selector.Plans)
			}
			return m, nil
		case "k", "up":
			if len(m.selector.Plans) > 0 {
				m.selector.PlanSel = (m.selector.PlanSel - 1 + len(m.selector.Plans)) % len(m.selector.Plans)
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
				m.planWiz.ActionMsg = fmt.Sprintf("Cannot switch to %q plan", status)
				return m, nil
			}
			if err := m.store.SetActivePlan(p.ID); err != nil {
				m.planWiz.ActionMsg = "Switch failed: " + err.Error()
				return m, nil
			}
			m.loadProjectData()
			m.planWiz.ActionMsg = "Active plan set to " + p.ID
			return m, nil
		case "n":
			if err := m.store.SetActivePlan(""); err != nil {
				m.planWiz.ActionMsg = "Clear failed: " + err.Error()
				return m, nil
			}
			m.loadProjectData()
			m.planWiz.ActionMsg = "Active plan cleared"
			return m, nil
		case "c":
			m.planWiz.CreateIDInput = ""
			m.planWiz.CreateTitleInput = ""
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
				m.planWiz.ActionMsg = "Only done/cancelled plans can be deleted"
				return m, nil
			}
			if err := m.store.DeletePlan(p.ID); err != nil {
				m.planWiz.ActionMsg = "Delete failed: " + err.Error()
				return m, nil
			}
			m.loadProjectData()
			if m.selector.PlanSel >= len(m.selector.Plans) && m.selector.PlanSel > 0 {
				m.selector.PlanSel--
			}
			m.planWiz.ActionMsg = "Deleted plan " + p.ID
			return m, nil
		case "r":
			m.loadProjectData()
			m.planWiz.ActionMsg = "Refreshed"
			return m, nil
		}
	}
	return m, nil
}

func (m AppModel) updatePlanCreateID(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("plan-create-id", m.planWiz.CreateIDInput, 64)
	m.syncTextInput(m.planWiz.CreateIDInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			id := strings.TrimSpace(m.planWiz.CreateIDInput)
			if id == "" {
				return m, nil
			}
			if !tuiPlanIDPattern.MatchString(id) {
				m.planWiz.ActionMsg = "Invalid ID format"
				return m, nil
			}
			if existing, _ := m.store.GetPlan(id); existing != nil {
				m.planWiz.ActionMsg = "Plan already exists"
				return m, nil
			}
			m.planWiz.CreateIDInput = id
			m.planWiz.CreateTitleInput = ""
			m.state = statePlanCreateTitle
			return m, nil
		case tea.KeyEsc:
			m.state = statePlanMenu
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.planWiz.CreateIDInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) updatePlanCreateTitle(msg tea.Msg) (tea.Model, tea.Cmd) {
	initCmd := m.ensureTextInput("plan-create-title", m.planWiz.CreateTitleInput, 0)
	m.syncTextInput(m.planWiz.CreateTitleInput)
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEnter:
			title := strings.TrimSpace(m.planWiz.CreateTitleInput)
			if title == "" {
				return m, nil
			}
			plan := &store.Plan{
				ID:     m.planWiz.CreateIDInput,
				Title:  title,
				Status: "active",
			}
			if err := m.store.CreatePlan(plan); err != nil {
				m.planWiz.ActionMsg = "Create failed: " + err.Error()
				return m, nil
			}
			if m.project != nil && strings.TrimSpace(m.project.ActivePlanID) == "" {
				_ = m.store.SetActivePlan(plan.ID)
			}
			m.loadProjectData()
			m.planWiz.ActionMsg = "Created plan " + plan.ID
			m.planWiz.CreateIDInput = ""
			m.planWiz.CreateTitleInput = ""
			m.state = statePlanMenu
			for i, p := range m.selector.Plans {
				if p.ID == plan.ID {
					m.selector.PlanSel = i
					break
				}
			}
			return m, nil
		case tea.KeyEsc:
			m.state = statePlanCreateID
			return m, nil
		}
	}
	cmd := m.updateTextInput(msg)
	m.planWiz.CreateTitleInput = m.textInput.Value()
	return m, tea.Batch(initCmd, cmd)
}

func (m AppModel) selectedPlan() *store.Plan {
	if m.selector.PlanSel < 0 || m.selector.PlanSel >= len(m.selector.Plans) {
		return nil
	}
	p := m.selector.Plans[m.selector.PlanSel]
	return &p
}

func (m AppModel) applySelectedPlanStatus(status string) (tea.Model, tea.Cmd) {
	p := m.selectedPlan()
	if p == nil {
		return m, nil
	}
	msg, err := m.applyPlanStatus(p.ID, status)
	if err != nil {
		m.planWiz.ActionMsg = err.Error()
		return m, nil
	}
	m.planWiz.ActionMsg = msg
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
	for i, p := range m.selector.Plans {
		if p.ID == planID {
			m.selector.PlanSel = i
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
	for _, p := range m.selector.Plans {
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

	if len(m.selector.Plans) == 0 {
		lines = append(lines, dimStyle.Render("No plans yet. Press c to create one."))
	} else {
		for i, p := range m.selector.Plans {
			status := p.Status
			if status == "" {
				status = "active"
			}
			prefix := "  "
			if i == m.selector.PlanSel {
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
			if i == m.selector.PlanSel {
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
	if m.planWiz.ActionMsg != "" {
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Last: "+truncateInputForDisplay(m.planWiz.ActionMsg, cw-6)))
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
	m.ensureTextInput("plan-create-id", m.planWiz.CreateIDInput, 64)
	m.syncTextInput(m.planWiz.CreateIDInput)

	var lines []string
	lines = append(lines, sectionStyle.Render("New Plan — ID"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Enter plan ID (lowercase, digits, -, _; max 64):"))
	lines = append(lines, "")
	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: next  esc: cancel"))
	if m.planWiz.ActionMsg != "" {
		lines = append(lines, dimStyle.Render("Last: "+truncateInputForDisplay(m.planWiz.ActionMsg, cw-6)))
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
	m.ensureTextInput("plan-create-title", m.planWiz.CreateTitleInput, 0)
	m.syncTextInput(m.planWiz.CreateTitleInput)

	var lines []string
	lines = append(lines, sectionStyle.Render("New Plan — Title"))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("Plan ID: "+m.planWiz.CreateIDInput))
	lines = append(lines, dimStyle.Render("Enter a title:"))
	lines = append(lines, "")
	lines = append(lines, m.viewTextInput(cw-4))
	lines = append(lines, "")
	lines = append(lines, dimStyle.Render("enter: create  esc: back"))
	if m.planWiz.ActionMsg != "" {
		lines = append(lines, dimStyle.Render("Last: "+truncateInputForDisplay(m.planWiz.ActionMsg, cw-6)))
	}

	content := fitLines(lines, cw, ch)
	panel := style.Render(content)
	return header + "\n" + panel + "\n" + statusBar
}
