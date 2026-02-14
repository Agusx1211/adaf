package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/debug"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/store"
)

var pmCmd = &cobra.Command{
	Use:   "pm [message]",
	Short: "Run a single project manager session with a message and exit",
	Long: `Run a standalone Project Manager session. The selected agent acts as a
project manager for one turn: it reviews context, updates planning artifacts,
and responds to your message.

Examples:
  adaf pm "Review plan progress and prioritize open issues"
  adaf pm --agent codex --model gpt-5.1-codex-max "Prepare next sprint"
  echo "Summarize blockers and propose next steps" | adaf pm
  adaf pm -s "Draft plan updates for today's work"`,
	RunE: runPM,
}

func init() {
	pmCmd.Flags().String("agent", "claude", "Agent to use (claude, codex, vibe, opencode, gemini, generic)")
	pmCmd.Flags().String("profile", "", "Use a named profile instead of --agent/--model")
	pmCmd.Flags().String("model", "", "Model override for the agent")
	pmCmd.Flags().String("plan", "", "Plan ID override (defaults to active plan)")
	pmCmd.Flags().BoolP("session", "s", false, "Start detached (use 'adaf attach' to connect)")
	rootCmd.AddCommand(pmCmd)
}

func runPM(cmd *cobra.Command, args []string) error {
	debug.Log("cli.pm", "runPM() called")
	if session.IsAgentContext() {
		return fmt.Errorf("pm is not available inside an agent context")
	}

	message, err := resolvePMMessage(args)
	if err != nil {
		return err
	}

	sessionMode, _ := cmd.Flags().GetBool("session")

	s, err := openStoreRequired()
	if err != nil {
		return err
	}

	prof, globalCfg, commandOverride, err := resolvePMProfile(cmd)
	if err != nil {
		return err
	}

	projCfg, err := s.LoadProject()
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	planFlag, _ := cmd.Flags().GetString("plan")
	effectivePlanID, err := resolveEffectivePlanID(s, projCfg, planFlag, cmd.Flags().Changed("plan"))
	if err != nil {
		return err
	}

	workDir := projCfg.RepoPath
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	fullPrompt, err := buildPMPrompt(s, projCfg, effectivePlanID, prof, globalCfg, message)
	if err != nil {
		return err
	}

	loopDef := config.LoopDef{
		Name: "pm",
		Steps: []config.LoopStep{
			{
				Profile:      prof.Name,
				Turns:        1,
				Instructions: fullPrompt,
			},
		},
	}
	maxCycles := 1

	var commandOverrides map[string]string
	if commandOverride != "" {
		commandOverrides = map[string]string{
			prof.Agent: commandOverride,
		}
	}

	dcfg := session.DaemonConfig{
		ProjectDir:            workDir,
		ProjectName:           projCfg.Name,
		WorkDir:               workDir,
		PlanID:                effectivePlanID,
		ProfileName:           prof.Name,
		AgentName:             prof.Agent,
		Loop:                  loopDef,
		Profiles:              []config.Profile{*prof},
		MaxCycles:             maxCycles,
		AgentCommandOverrides: commandOverrides,
	}

	sessionID, err := session.CreateSession(dcfg)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	debug.LogKV("cli.pm", "session created",
		"session_id", sessionID,
		"agent", prof.Agent,
	)

	if err := session.StartDaemon(sessionID); err != nil {
		debug.LogKV("cli.pm", "daemon start failed", "session_id", sessionID, "error", err)
		return fmt.Errorf("starting session daemon: %w", err)
	}

	if sessionMode {
		fmt.Printf("\n  %sSession #%d started%s (agent=%s, project=%s)\n",
			styleBoldGreen, sessionID, colorReset, prof.Agent, projCfg.Name)
		fmt.Printf("  Use %sadaf attach %d%s to connect.\n\n", styleBoldWhite, sessionID, colorReset)
		return nil
	}

	_, exitCode, err := streamAskSession(cmd.Context(), sessionID, projCfg.Name, prof.Agent, effectivePlanID, 1, 1)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

func resolvePMMessage(args []string) (string, error) {
	if len(args) > 0 {
		if joined := strings.TrimSpace(strings.Join(args, " ")); joined != "" {
			return joined, nil
		}
	}

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading message from stdin: %w", err)
		}
		if s := strings.TrimSpace(string(data)); s != "" {
			return s, nil
		}
	}

	return "", fmt.Errorf("no message provided — pass as argument or pipe via stdin")
}

func resolvePMProfile(cmd *cobra.Command) (*config.Profile, *config.GlobalConfig, string, error) {
	prof, globalCfg, cmdOverride, err := resolveProfile(cmd, ProfileResolveOpts{
		Prefix: "pm",
	})
	return prof, globalCfg, cmdOverride, err
}

func buildPMPrompt(s *store.Store, projCfg *store.ProjectConfig, planID string, prof *config.Profile, globalCfg *config.GlobalConfig, userMessage string) (string, error) {
	basePrompt, err := promptpkg.Build(promptpkg.BuildOpts{
		Store:     s,
		Project:   projCfg,
		PlanID:    planID,
		Profile:   prof,
		Role:      config.RoleManager,
		GlobalCfg: globalCfg,
	})
	if err != nil {
		return "", fmt.Errorf("building PM prompt: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Role: Project Manager\n\n")
	b.WriteString("You manage project execution through plans, issues, and documentation.\n")
	b.WriteString("You do NOT write code or directly edit implementation files.\n")
	b.WriteString("Keep outcomes concrete, prioritized, and actionable.\n\n")

	b.WriteString("## Your Capabilities\n\n")
	b.WriteString("- Plan management: `adaf plan ...`\n")
	b.WriteString("- Issue management: `adaf issue ...`\n")
	b.WriteString("- Documentation management: `adaf doc ...`\n")
	b.WriteString("- Project overview: `adaf status`\n")
	b.WriteString("- Historical context: `adaf log`\n\n")

	b.WriteString(basePrompt)
	b.WriteString("\n\n## User Message\n\n")
	b.WriteString(strings.TrimSpace(userMessage))
	b.WriteString("\n")

	return b.String(), nil
}
