package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/store"
)

const selectorLeftWidth = 24

// agentEntry holds display info for an agent in the selector list.
type agentEntry struct {
	Name     string
	Detected bool
	Path     string
	Model    string
	Caps     []string
}

// buildAgentList builds a sorted list of agents from the registry and config.
func buildAgentList(agentsCfg *agent.AgentsConfig) []agentEntry {
	all := agent.All()
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]agentEntry, 0, len(names))
	for _, name := range names {
		e := agentEntry{Name: name}
		if agentsCfg != nil {
			if rec, ok := agentsCfg.Agents[name]; ok {
				e.Detected = rec.Detected
				e.Path = rec.Path
				e.Model = rec.DefaultModel
				e.Caps = rec.Capabilities
			}
		}
		if e.Model == "" {
			e.Model = agent.DefaultModel(name)
		}
		if len(e.Caps) == 0 {
			e.Caps = agent.Capabilities(name)
		}
		entries = append(entries, e)
	}
	return entries
}

// renderSelector renders the selector view as two columns.
func renderSelector(agents []agentEntry, selected int, project *store.ProjectConfig, plan *store.Plan, issues []store.Issue, logs []store.SessionLog, width, height int) string {
	panelH := height - 2 // header + status bar
	if panelH < 1 {
		panelH = 1
	}

	leftOuter := selectorLeftWidth
	rightOuter := width - leftOuter
	if rightOuter < 20 {
		rightOuter = width
		leftOuter = 0
	}

	left := renderAgentList(agents, selected, leftOuter, panelH)
	right := renderProjectPanel(agents, selected, project, plan, issues, logs, rightOuter, panelH)

	if leftOuter == 0 {
		return right
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func renderAgentList(agents []agentEntry, selected int, outerW, outerH int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSurface2).
		Padding(1, 1)

	hf, vf := style.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorLavender).Render("Agents"))
	lines = append(lines, "")

	for i, a := range agents {
		name := a.Name
		maxW := cw - 4
		if maxW > 0 && len(name) > maxW {
			name = name[:maxW]
		}

		var indicator string
		if a.Detected {
			indicator = lipgloss.NewStyle().Foreground(ColorGreen).Render("  ")
		} else {
			indicator = lipgloss.NewStyle().Foreground(ColorOverlay0).Render("  ")
		}

		if i == selected {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+nameStyled+" "+indicator)
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(ColorText).Render(name)
			lines = append(lines, "  "+nameStyled+" "+indicator)
		}
	}

	content := fitLines(lines, cw, ch)
	return style.Render(content)
}

func renderProjectPanel(agents []agentEntry, selected int, project *store.ProjectConfig, plan *store.Plan, issues []store.Issue, logs []store.SessionLog, outerW, outerH int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSurface2).
		Padding(1, 1)

	hf, vf := style.GetFrameSize()
	cw := outerW - hf
	ch := outerH - vf
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Width(14)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorLavender)
	dimStyle := lipgloss.NewStyle().Foreground(ColorOverlay0)

	var lines []string

	// Project info section
	lines = append(lines, sectionStyle.Render("Project"))
	if project != nil {
		lines = append(lines, labelStyle.Render("Name")+valueStyle.Render(project.Name))
		if project.RepoPath != "" {
			path := project.RepoPath
			maxPathW := cw - 14
			if maxPathW > 0 && len(path) > maxPathW {
				path = "..." + path[len(path)-maxPathW+3:]
			}
			lines = append(lines, labelStyle.Render("Repo")+valueStyle.Render(path))
		}
	}

	// Plan progress
	if plan != nil && len(plan.Phases) > 0 {
		done := 0
		for _, p := range plan.Phases {
			if p.Status == "complete" {
				done++
			}
		}
		lines = append(lines, labelStyle.Render("Plan")+valueStyle.Render(
			fmt.Sprintf("%d/%d phases done", done, len(plan.Phases))))
	}

	// Issues count
	openCount := 0
	for _, iss := range issues {
		if iss.Status == "open" || iss.Status == "in_progress" {
			openCount++
		}
	}
	if openCount > 0 {
		lines = append(lines, labelStyle.Render("Issues")+
			lipgloss.NewStyle().Foreground(ColorYellow).Render(fmt.Sprintf("%d open", openCount)))
	} else {
		lines = append(lines, labelStyle.Render("Issues")+dimStyle.Render("none"))
	}

	// Sessions count
	lines = append(lines, labelStyle.Render("Sessions")+valueStyle.Render(fmt.Sprintf("%d", len(logs))))

	lines = append(lines, "")

	// Selected agent detail
	if selected >= 0 && selected < len(agents) {
		a := agents[selected]
		lines = append(lines, sectionStyle.Render("Selected Agent"))
		lines = append(lines, labelStyle.Render("Name")+valueStyle.Render(a.Name))
		if a.Model != "" {
			lines = append(lines, labelStyle.Render("Model")+valueStyle.Render(a.Model))
		}
		if a.Path != "" {
			path := a.Path
			maxPathW := cw - 14
			if maxPathW > 0 && len(path) > maxPathW {
				path = "..." + path[len(path)-maxPathW+3:]
			}
			lines = append(lines, labelStyle.Render("Path")+valueStyle.Render(path))
		}
		if a.Detected {
			lines = append(lines, labelStyle.Render("Status")+
				lipgloss.NewStyle().Foreground(ColorGreen).Render("detected"))
		} else {
			lines = append(lines, labelStyle.Render("Status")+dimStyle.Render("not found"))
		}
		if len(a.Caps) > 0 {
			lines = append(lines, labelStyle.Render("Capabilities")+dimStyle.Render(strings.Join(a.Caps, ", ")))
		}
	}

	content := fitLines(lines, cw, ch)
	return style.Render(content)
}

// fitLines is equivalent to runtui's fitToSize: exactly w cols and h lines.
func fitLines(lines []string, w, h int) string {
	truncator := lipgloss.NewStyle().MaxWidth(w)
	emptyLine := strings.Repeat(" ", w)
	result := make([]string, h)

	for i := range h {
		if i < len(lines) {
			line := lines[i]
			lw := lipgloss.Width(line)
			if lw > w {
				line = truncator.Render(line)
				lw = lipgloss.Width(line)
			}
			if pad := w - lw; pad > 0 {
				line += strings.Repeat(" ", pad)
			}
			result[i] = line
		} else {
			result[i] = emptyLine
		}
	}
	return strings.Join(result, "\n")
}
