package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/agusx1211/adaf/internal/agent"
	cfgpkg "github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

const selectorLeftWidth = 28

// profileEntry holds display info for a profile or loop in the selector list.
type profileEntry struct {
	Name            string
	Agent           string
	Model           string
	ReasoningLevel  string // thinking budget value from profile
	Role            string // "manager", "senior", "junior", "supervisor"
	Intelligence    int
	Description     string
	MaxInstances    int // max concurrent instances (0 = unlimited)
	Detected        bool
	IsNew           bool              // sentinel "+ New Profile" entry
	IsNewLoop       bool              // sentinel "+ New Loop" entry
	IsLoop          bool              // true if this represents a loop definition
	LoopName        string            // loop name (when IsLoop)
	LoopSteps       int               // number of steps (when IsLoop)
	LoopStepDetails []cfgpkg.LoopStep // step details for rendering (when IsLoop)
	IsSeparator     bool              // separator line between sections
	Caps            []string
}

// buildProfileList builds a list from saved profiles, loops, and sentinel entries.
func buildProfileList(globalCfg *cfgpkg.GlobalConfig, agentsCfg *agent.AgentsConfig) []profileEntry {
	entries := make([]profileEntry, 0, len(globalCfg.Profiles)+len(globalCfg.Loops)+4)
	for _, p := range globalCfg.Profiles {
		e := profileEntry{
			Name:           p.Name,
			Agent:          p.Agent,
			Model:          p.Model,
			ReasoningLevel: p.ReasoningLevel,
			Role:           cfgpkg.EffectiveRole(p.Role),
			Intelligence:   p.Intelligence,
			Description:    p.Description,
			MaxInstances:   p.MaxInstances,
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

	// Add loops if any exist.
	if len(globalCfg.Loops) > 0 {
		entries = append(entries, profileEntry{IsSeparator: true, Name: "───"})
		for _, l := range globalCfg.Loops {
			entries = append(entries, profileEntry{
				IsLoop:          true,
				LoopName:        l.Name,
				Name:            l.Name,
				LoopSteps:       len(l.Steps),
				LoopStepDetails: l.Steps,
			})
		}
	}

	entries = append(entries, profileEntry{IsSeparator: true, Name: "───"})
	entries = append(entries, profileEntry{IsNew: true, Name: "+ New Profile"})
	entries = append(entries, profileEntry{IsNewLoop: true, Name: "+ New Loop"})
	return entries
}

// renderSelector renders the selector view as two columns.
func renderSelector(profiles []profileEntry, selected int, project *store.ProjectConfig, plan *store.Plan, issues []store.Issue, logs []store.SessionLog, profileStats map[string]*store.ProfileStats, loopStats map[string]*store.LoopStats, width, height int) string {
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
	right := renderProjectPanel(profiles, selected, project, plan, issues, logs, profileStats, loopStats, rightOuter, panelH)

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
		// Separator line.
		if p.IsSeparator {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorOverlay0).Render("  "+p.Name))
			continue
		}

		name := p.Name
		maxW := cw - 10 // leave room for role/loop badge
		if maxW > 0 && len(name) > maxW {
			name = name[:maxW]
		}

		if p.IsNew || p.IsNewLoop {
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

		if p.IsLoop {
			badge := lipgloss.NewStyle().Foreground(ColorTeal).Render("[LOOP]")
			if i == selected {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render("> ")
				nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorTeal).Render(name)
				lines = append(lines, cursor+nameStyled+" "+badge)
			} else {
				nameStyled := lipgloss.NewStyle().Foreground(ColorText).Render(name)
				lines = append(lines, "  "+nameStyled+" "+badge)
			}
			continue
		}

		badge := roleBadge(p.Role)

		if i == selected {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+nameStyled+" "+badge)
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(ColorText).Render(name)
			lines = append(lines, "  "+nameStyled+" "+badge)
		}
	}

	content := fitLines(lines, cw, ch)
	return style.Render(content)
}

func renderProjectPanel(profiles []profileEntry, selected int, project *store.ProjectConfig, plan *store.Plan, issues []store.Issue, logs []store.SessionLog, profileStats map[string]*store.ProfileStats, loopStats map[string]*store.LoopStats, outerW, outerH int) string {
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

	// Selected profile/loop detail
	if selected >= 0 && selected < len(profiles) {
		p := profiles[selected]
		if p.IsSeparator {
			// No detail for separator.
		} else if p.IsNewLoop {
			lines = append(lines, sectionStyle.Render("New Loop"))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("Press enter or l to create"))
			lines = append(lines, dimStyle.Render("a new loop definition."))
		} else if p.IsNew {
			lines = append(lines, sectionStyle.Render("New Profile"))
			lines = append(lines, "")
			lines = append(lines, dimStyle.Render("Press enter or n to create"))
			lines = append(lines, dimStyle.Render("a new agent profile."))
		} else if p.IsLoop {
			lines = append(lines, sectionStyle.Render("Selected Loop"))
			lines = append(lines, labelStyle.Render("Loop")+valueStyle.Render(p.LoopName))
			lines = append(lines, labelStyle.Render("Steps")+valueStyle.Render(fmt.Sprintf("%d", p.LoopSteps)))
			// Step details.
			if len(p.LoopStepDetails) > 0 {
				lines = append(lines, "")
				lines = append(lines, sectionStyle.Render("Step Details"))
				for i, step := range p.LoopStepDetails {
					turns := step.Turns
					if turns <= 0 {
						turns = 1
					}
					flags := ""
					if step.CanStop {
						flags += " [stop]"
					}
					if step.CanMessage {
						flags += " [msg]"
					}
					if step.CanPushover {
						flags += " [push]"
					}
					stepLine := fmt.Sprintf("%d. %s x%d%s", i+1, step.Profile, turns, flags)
					lines = append(lines, dimStyle.Render(stepLine))
					if step.Instructions != "" {
						instr := step.Instructions
						maxInstrW := cw - 4
						if maxInstrW > 0 && len(instr) > maxInstrW {
							instr = instr[:maxInstrW-3] + "..."
						}
						lines = append(lines, dimStyle.Render("  "+instr))
					}
				}
			}
			// Loop stats
			if ls, ok := loopStats[p.LoopName]; ok && ls.TotalRuns > 0 {
				lines = append(lines, "")
				lines = append(lines, sectionStyle.Render("Stats"))
				lines = append(lines, labelStyle.Render("Runs")+valueStyle.Render(fmt.Sprintf("%d", ls.TotalRuns)))
				lines = append(lines, labelStyle.Render("Cycles")+valueStyle.Render(fmt.Sprintf("%d", ls.TotalCycles)))
				lines = append(lines, labelStyle.Render("Cost")+valueStyle.Render(fmt.Sprintf("$%.2f", ls.TotalCostUSD)))
				if !ls.LastRunAt.IsZero() {
					lines = append(lines, labelStyle.Render("Last")+dimStyle.Render(formatSelectorTimeAgo(ls.LastRunAt)))
				}
			}
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
			lines = append(lines, labelStyle.Render("Role")+valueStyle.Render(p.Role)+" "+roleBadge(p.Role))
			if p.Intelligence > 0 {
				lines = append(lines, labelStyle.Render("Intelligence")+valueStyle.Render(fmt.Sprintf("%d/10", p.Intelligence)))
			}
			if p.MaxInstances > 0 {
				lines = append(lines, labelStyle.Render("Max Instances")+valueStyle.Render(fmt.Sprintf("%d", p.MaxInstances)))
			} else {
				lines = append(lines, labelStyle.Render("Max Instances")+dimStyle.Render("unlimited"))
			}
			if p.Description != "" {
				desc := p.Description
				maxDescW := cw - 14
				if maxDescW > 0 && len(desc) > maxDescW {
					desc = desc[:maxDescW-3] + "..."
				}
				lines = append(lines, labelStyle.Render("Description")+dimStyle.Render(desc))
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
			// Profile stats
			if ps, ok := profileStats[p.Name]; ok && ps.TotalRuns > 0 {
				lines = append(lines, "")
				lines = append(lines, sectionStyle.Render("Stats"))
				lines = append(lines, labelStyle.Render("Runs")+valueStyle.Render(
					fmt.Sprintf("%d (%d ok, %d fail)", ps.TotalRuns, ps.SuccessCount, ps.FailureCount)))
				lines = append(lines, labelStyle.Render("Cost")+valueStyle.Render(fmt.Sprintf("$%.2f", ps.TotalCostUSD)))
				if !ps.LastRunAt.IsZero() {
					lines = append(lines, labelStyle.Render("Last")+dimStyle.Render(formatSelectorTimeAgo(ps.LastRunAt)))
				}
				if len(ps.ToolCalls) > 0 {
					lines = append(lines, labelStyle.Render("Top tools")+dimStyle.Render(formatSelectorTopTools(ps.ToolCalls)))
				}
			}
		}
	}

	content := fitLines(lines, cw, ch)
	return style.Render(content)
}

// roleBadge returns a styled role badge like [MGR], [SR], [JR], [SUP].
func roleBadge(role string) string {
	var tag string
	var color lipgloss.TerminalColor
	switch role {
	case "manager":
		tag = "MGR"
		color = ColorPeach
	case "senior":
		tag = "SR"
		color = ColorBlue
	case "junior":
		tag = "JR"
		color = ColorGreen
	case "supervisor":
		tag = "SUP"
		color = ColorYellow
	default:
		tag = "JR"
		color = ColorOverlay0
	}
	return lipgloss.NewStyle().Foreground(color).Render("[" + tag + "]")
}

// truncateInputForDisplay returns the tail of the input string that fits within
// maxWidth, prefixed with "..." if truncated. This prevents text inputs from
// overflowing outside the terminal panel.
func truncateInputForDisplay(input string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if len(input) <= maxWidth {
		return input
	}
	if maxWidth <= 3 {
		return input[len(input)-maxWidth:]
	}
	return "..." + input[len(input)-maxWidth+3:]
}

// fitLines is equivalent to runtui's fitToSize: exactly w cols and h lines.
func fitLines(lines []string, w, h int) string {
	emptyLine := strings.Repeat(" ", w)
	result := make([]string, h)

	for i := 0; i < h; i++ {
		if i < len(lines) {
			line := lines[i]
			parts := splitRenderableLines(line)
			if len(parts) > 0 {
				line = parts[0]
			}
			line = ansi.Truncate(line, w, "")
			lw := lipgloss.Width(line)
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

func splitRenderableLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	if len(parts) == 0 {
		return []string{""}
	}
	return parts
}

// formatSelectorTimeAgo returns a short human-readable time-ago string for the TUI.
func formatSelectorTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd ago", days)
	}
}

// formatSelectorTopTools returns a compact top-tools string for the TUI.
func formatSelectorTopTools(tools map[string]int) string {
	type tc struct {
		name  string
		count int
	}
	var sorted []tc
	for name, count := range tools {
		sorted = append(sorted, tc{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	var parts []string
	for i, t := range sorted {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s(%d)", t.name, t.count))
	}
	return strings.Join(parts, " ")
}
