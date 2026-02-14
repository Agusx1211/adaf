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
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

const selectorLeftWidth = 28

type SelectorState struct {
	Selected       int
	Plans          []store.Plan
	PlanSel        int
	Msg            string
	SessionPickSel int
}

// profileEntry holds display info for a profile or loop in the selector list.
type profileEntry struct {
	Name            string
	Agent           string
	Model           string
	ReasoningLevel  string // thinking budget value from profile
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
			steps := make([]cfgpkg.LoopStep, len(l.Steps))
			for i, step := range l.Steps {
				steps[i] = step
				steps[i].Role = cfgpkg.EffectiveStepRole(step.Role, globalCfg)
			}
			entries = append(entries, profileEntry{
				IsLoop:          true,
				LoopName:        l.Name,
				Name:            l.Name,
				LoopSteps:       len(l.Steps),
				LoopStepDetails: steps,
			})
		}
	}

	entries = append(entries, profileEntry{IsSeparator: true, Name: "───"})
	entries = append(entries, profileEntry{IsNew: true, Name: "+ New Profile"})
	entries = append(entries, profileEntry{IsNewLoop: true, Name: "+ New Loop"})
	return entries
}

// renderSelector renders the selector view as two columns.
func renderSelector(
	profiles []profileEntry,
	selected int,
	project *store.ProjectConfig,
	plan *store.Plan,
	plans []store.Plan,
	issues []store.Issue,
	docs []store.Doc,
	turns []store.Turn,
	activeSessions []session.SessionMeta,
	activeLoop *store.LoopRun,
	profileStats map[string]*store.ProfileStats,
	loopStats map[string]*store.LoopStats,
	width, height int,
	rightScroll int,
	rightFocused bool,
) string {
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

	left := renderProfileList(profiles, selected, leftOuter, panelH, !rightFocused)
	right := renderProjectPanel(
		profiles,
		selected,
		project,
		plan,
		plans,
		issues,
		docs,
		turns,
		activeSessions,
		activeLoop,
		profileStats,
		loopStats,
		rightOuter,
		panelH,
		rightScroll,
		rightFocused,
	)

	if leftOuter == 0 {
		return right
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func renderProfileList(profiles []profileEntry, selected int, outerW, outerH int, focused bool) string {
	border := ColorSurface2
	if focused {
		border = ColorMauve
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
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
	cursorLine := -1
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorLavender).Render("Profiles"))
	lines = append(lines, "")

	for i, p := range profiles {
		// Separator line.
		if p.IsSeparator {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorOverlay0).Render("  "+p.Name))
			continue
		}

		name := p.Name
		maxW := cw - 10 // leave room for loop badge
		if maxW > 0 && len(name) > maxW {
			name = name[:maxW]
		}

		if p.IsNew || p.IsNewLoop {
			// Sentinel entry styled differently.
			if i == selected {
				cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("> ")
				nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render(name)
				lines = append(lines, cursor+nameStyled)
				cursorLine = len(lines) - 1
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
				cursorLine = len(lines) - 1
			} else {
				nameStyled := lipgloss.NewStyle().Foreground(ColorText).Render(name)
				lines = append(lines, "  "+nameStyled+" "+badge)
			}
			continue
		}

		if i == selected {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render("> ")
			nameStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Render(name)
			lines = append(lines, cursor+nameStyled)
			cursorLine = len(lines) - 1
		} else {
			nameStyled := lipgloss.NewStyle().Foreground(ColorText).Render(name)
			lines = append(lines, "  "+nameStyled)
		}
	}

	content := fitLinesWithCursor(lines, cw, ch, cursorLine)
	return style.Render(content)
}

func renderProjectPanel(
	profiles []profileEntry,
	selected int,
	project *store.ProjectConfig,
	plan *store.Plan,
	plans []store.Plan,
	issues []store.Issue,
	docs []store.Doc,
	turns []store.Turn,
	activeSessions []session.SessionMeta,
	activeLoop *store.LoopRun,
	profileStats map[string]*store.ProfileStats,
	loopStats map[string]*store.LoopStats,
	outerW, outerH int,
	scrollOffset int,
	focused bool,
) string {
	border := ColorSurface2
	if focused {
		border = ColorMauve
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
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
		activeID := strings.TrimSpace(project.ActivePlanID)
		if activeID == "" {
			lines = append(lines, labelStyle.Render("Active Plan")+dimStyle.Render("(none)"))
		} else {
			lines = append(lines, labelStyle.Render("Active Plan")+valueStyle.Render(activeID))
		}
	}

	// Plan progress and overview.
	if plan != nil {
		done := 0
		for _, p := range plan.Phases {
			if p.Status == "complete" {
				done++
			}
		}
		lines = append(lines, labelStyle.Render("Plan")+valueStyle.Render(
			fmt.Sprintf("%s (%d/%d phases)", plan.ID, done, len(plan.Phases))))
	}
	if len(plans) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionStyle.Render("Plans"))
		limit := len(plans)
		if limit > 5 {
			limit = 5
		}
		activeID := ""
		if project != nil {
			activeID = strings.TrimSpace(project.ActivePlanID)
		}
		for i := 0; i < limit; i++ {
			p := plans[i]
			id := p.ID
			if p.ID == activeID {
				id = "● " + id
			}
			status := p.Status
			if status == "" {
				status = "active"
			}
			statusText := selectorRuntimeStatusStyle(status).Render("[" + status + "]")
			title := p.Title
			if title == "" {
				title = "(untitled)"
			}
			lines = append(lines, "  "+valueStyle.Render(truncateInputForDisplay(id+" "+title, cw-16))+" "+statusText)
		}
		if len(plans) > limit {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... +%d more", len(plans)-limit)))
		}
	}

	// Issues split by status
	openIssues := make([]store.Issue, 0, len(issues))
	resolvedIssues := make([]store.Issue, 0, len(issues))
	for _, iss := range issues {
		switch iss.Status {
		case "resolved", "wontfix":
			resolvedIssues = append(resolvedIssues, iss)
		case "open", "in_progress":
			openIssues = append(openIssues, iss)
		default:
			openIssues = append(openIssues, iss)
		}
	}

	lines = append(lines, labelStyle.Render("Issues")+valueStyle.Render(fmt.Sprintf("%d", len(issues))))
	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Open Issues"))
	if len(openIssues) == 0 {
		lines = append(lines, dimStyle.Render("  none"))
	} else {
		for _, iss := range openIssues {
			lines = append(lines, fmt.Sprintf("  #%d %s [%s]", iss.ID, iss.Title, iss.Status))
		}
	}
	if len(resolvedIssues) > 0 {
		lines = append(lines, "")
		lines = append(lines, sectionStyle.Render("Resolved"))
		for _, iss := range resolvedIssues {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  #%d %s [%s]", iss.ID, iss.Title, iss.Status)))
		}
	}

	// Turns count
	lines = append(lines, labelStyle.Render("Turns")+valueStyle.Render(fmt.Sprintf("%d", len(turns))))
	lines = append(lines, labelStyle.Render("Docs")+valueStyle.Render(fmt.Sprintf("%d", len(docs))))

	lines = append(lines, "")
	lines = append(lines, sectionStyle.Render("Runtime"))
	if len(activeSessions) == 0 {
		lines = append(lines, labelStyle.Render("Sessions")+dimStyle.Render("none running"))
	} else {
		lines = append(lines, labelStyle.Render("Sessions")+valueStyle.Render(fmt.Sprintf("%d active", len(activeSessions))))
		limit := len(activeSessions)
		if limit > 4 {
			limit = 4
		}
		for i := 0; i < limit; i++ {
			s := activeSessions[i]
			label := fmt.Sprintf("#%d %s/%s", s.ID, s.ProfileName, s.AgentName)
			statusText := selectorRuntimeStatusStyle(s.Status).Render(s.Status)
			lines = append(lines, "  "+valueStyle.Render(truncateInputForDisplay(label, cw-16))+" "+statusText)
		}
		if len(activeSessions) > limit {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("  ... +%d more", len(activeSessions)-limit)))
		}
	}
	if activeLoop == nil || activeLoop.Status != "running" {
		lines = append(lines, labelStyle.Render("Loop")+dimStyle.Render("none running"))
	} else {
		stepDesc := "n/a"
		if activeLoop.StepIndex >= 0 && activeLoop.StepIndex < len(activeLoop.Steps) {
			step := activeLoop.Steps[activeLoop.StepIndex]
			stepDesc = fmt.Sprintf("%d/%d %s", activeLoop.StepIndex+1, len(activeLoop.Steps), step.Profile)
		}
		lines = append(lines, labelStyle.Render("Loop")+valueStyle.Render(fmt.Sprintf("#%d %s", activeLoop.ID, activeLoop.LoopName)))
		lines = append(lines, labelStyle.Render("Status")+selectorRuntimeStatusStyle(activeLoop.Status).Render(activeLoop.Status))
		lines = append(lines, labelStyle.Render("Cycle")+valueStyle.Render(fmt.Sprintf("%d", activeLoop.Cycle+1)))
		lines = append(lines, labelStyle.Render("Step")+dimStyle.Render(stepDesc))
	}

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
					role := strings.ToLower(strings.TrimSpace(step.Role))
					if role == "" {
						role = cfgpkg.RoleDeveloper
					}
					spawnCount := 0
					if step.Delegation != nil {
						spawnCount = len(step.Delegation.Profiles)
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
					spawnTag := " [no-spawn]"
					if spawnCount > 0 {
						spawnTag = fmt.Sprintf(" [spawn:%d]", spawnCount)
					}
					stepLine := fmt.Sprintf("%d. %s %s x%d%s%s", i+1, step.Profile, roleBadge(role), turns, spawnTag, flags)
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

	content := fitLinesWithOffset(lines, cw, ch, scrollOffset)
	return style.Render(content)
}

// roleBadge returns a styled role badge like [MGR], [LED], [DEV], [SUP].
func roleBadge(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	var tag string
	var color lipgloss.TerminalColor
	switch role {
	case "manager":
		tag = "MGR"
		color = ColorPeach
	case "lead-developer":
		tag = "LED"
		color = ColorBlue
	case "developer":
		tag = "DEV"
		color = ColorGreen
	case "supervisor":
		tag = "SUP"
		color = ColorYellow
	case "ui-designer":
		tag = "UI"
		color = ColorMauve
	case "qa":
		tag = "QA"
		color = ColorTeal
	case "backend-designer":
		tag = "BED"
		color = ColorBlue
	case "documentator":
		tag = "DOC"
		color = ColorLavender
	case "reviewer":
		tag = "REV"
		color = ColorRed
	case "scout":
		tag = "SCT"
		color = ColorTeal
	case "researcher":
		tag = "RSH"
		color = ColorFlamingo
	default:
		tag = strings.ToUpper(role)
		if len(tag) > 3 {
			tag = tag[:3]
		}
		if tag == "" {
			tag = "UNK"
		}
		color = ColorOverlay0
	}
	return lipgloss.NewStyle().Foreground(color).Render("[" + tag + "]")
}

func selectorRuntimeStatusStyle(status string) lipgloss.Style {
	switch status {
	case "running", "starting", "in_progress":
		return lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
	case "done", "stopped", "completed", "resolved":
		return lipgloss.NewStyle().Foreground(ColorGreen)
	case "cancelled", "failed", "dead", "error":
		return lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(ColorOverlay0)
	}
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
	return fitLinesWithOffset(lines, w, h, 0)
}

func fitLinesWithOffset(lines []string, w, h, offset int) string {
	return fitPreparedLines(prepareLinesForFit(lines), w, h, offset)
}

func fitLinesWithCursor(lines []string, w, h, cursorLine int) string {
	prepared := prepareLinesForFit(lines)
	return fitPreparedLines(prepared, w, h, offsetForCursor(cursorLine, len(prepared), h))
}

func prepareLinesForFit(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	prepared := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := splitRenderableLines(line)
		if len(parts) == 0 {
			prepared = append(prepared, "")
			continue
		}
		prepared = append(prepared, parts[0])
	}
	return prepared
}

func fitPreparedLines(lines []string, w, h, offset int) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	maxOffset := maxLineOffset(len(lines), h)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	emptyLine := strings.Repeat(" ", w)
	result := make([]string, h)

	for i := 0; i < h; i++ {
		lineIdx := offset + i
		if lineIdx < len(lines) {
			line := ansi.Truncate(lines[lineIdx], w, "")
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

func maxLineOffset(totalLines, height int) int {
	if totalLines <= height {
		return 0
	}
	return totalLines - height
}

func offsetForCursor(cursorLine, totalLines, height int) int {
	if cursorLine < 0 || totalLines <= height {
		return 0
	}
	offset := cursorLine - (height / 2)
	if offset < 0 {
		offset = 0
	}
	maxOffset := maxLineOffset(totalLines, height)
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

func wrapRenderableLines(lines []string, width int) []string {
	if len(lines) == 0 {
		return nil
	}
	if width < 1 {
		width = 1
	}

	out := make([]string, 0, len(lines))
	for _, line := range lines {
		for _, part := range splitRenderableLines(line) {
			wrapped := ansi.Wrap(part, width, " ")
			out = append(out, splitRenderableLines(wrapped)...)
		}
	}
	return out
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
