package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

const selectorLeftWidth = 28

// profileEntry holds display info for a profile in the selector list.
type profileEntry struct {
	Name          string
	Agent         string
	Model         string
	ReasoningLevel string // thinking budget value from profile
	Detected      bool
	IsNew         bool // sentinel "+ New Profile" entry
	Caps          []string
}

// buildProfileList builds a list from saved profiles + a "New Profile" sentinel.
func buildProfileList(globalCfg *config.GlobalConfig, agentsCfg *agent.AgentsConfig) []profileEntry {
	entries := make([]profileEntry, 0, len(globalCfg.Profiles)+1)
	for _, p := range globalCfg.Profiles {
		e := profileEntry{
			Name:          p.Name,
			Agent:         p.Agent,
			Model:         p.Model,
			ReasoningLevel: p.ReasoningLevel,
		}
		if e.Model == "" {
			e.Model = agent.DefaultModel(p.Agent)
		}
		e.Caps = agent.Capabilities(p.Agent)
		if agentsCfg != nil {
			if rec, ok := agentsCfg.Agents[p.Agent]; ok {
				e.Detected = rec.Detected
			}
		}
		entries = append(entries, e)
	}
	entries = append(entries, profileEntry{IsNew: true, Name: "+ New Profile"})
	return entries
}

// renderSelector renders the selector view as two columns.
func renderSelector(profiles []profileEntry, selected int, project *store.ProjectConfig, plan *store.Plan, issues []store.Issue, logs []store.SessionLog, width, height int) string {
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

	left := renderProfileList(profiles, selected, leftOuter, panelH)
	right := renderProjectPanel(profiles, selected, project, plan, issues, logs, rightOuter, panelH)

	if leftOuter == 0 {
		return right
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func renderProfileList(profiles []profileEntry, selected int, outerW, outerH int) string {
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
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorLavender).Render("Profiles"))
	lines = append(lines, "")

	for i, p := range profiles {
		name := p.Name
		maxW := cw - 4
		if maxW > 0 && len(name) > maxW {
			name = name[:maxW]
		}

		if p.IsNew {
			// Sentinel entry styled differently.
			if i == selected {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
				nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render(name)
				lines = append(lines, cursor+nameStyled)
			} else {
				nameStyled := lipgloss.NewStyle().Foreground(ColorOverlay0).Render(name)
				lines = append(lines, "  "+nameStyled)
			}
			continue
		}

		var indicator string
		if p.Detected {
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

func renderProjectPanel(profiles []profileEntry, selected int, project *store.ProjectConfig, plan *store.Plan, issues []store.Issue, logs []store.SessionLog, outerW, outerH int) string {
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

	// Selected profile detail
	if selected >= 0 && selected < len(profiles) {
		p := profiles[selected]
		if p.IsNew {
			lines = append(lines, sectionStyle.Render("New Profile"))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("Press enter or n to create"))
			lines = append(lines, dimStyle.Render("a new agent profile."))
		} else {
			lines = append(lines, sectionStyle.Render("Selected Profile"))
			lines = append(lines, labelStyle.Render("Profile")+valueStyle.Render(p.Name))
			lines = append(lines, labelStyle.Render("Agent")+valueStyle.Render(p.Agent))
			if p.Model != "" {
				lines = append(lines, labelStyle.Render("Model")+valueStyle.Render(p.Model))
			}
			if p.ReasoningLevel != "" {
				lines = append(lines, labelStyle.Render("Reasoning")+valueStyle.Render(p.ReasoningLevel))
			}
			if p.Detected {
				lines = append(lines, labelStyle.Render("Status")+
					lipgloss.NewStyle().Foreground(ColorGreen).Render("detected"))
			} else {
				lines = append(lines, labelStyle.Render("Status")+dimStyle.Render("not found"))
			}
			if len(p.Caps) > 0 {
				lines = append(lines, labelStyle.Render("Capabilities")+dimStyle.Render(strings.Join(p.Caps, ", ")))
			}
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
