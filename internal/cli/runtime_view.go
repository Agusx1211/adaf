package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/buildinfo"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/session"
)

type cliRuntimeView int

const (
	cliRuntimeViewUser cliRuntimeView = iota
	cliRuntimeViewAgent
)

type commandAudience int

const (
	commandAudienceBoth commandAudience = iota
	commandAudienceUserOnly
	commandAudienceAgentOnly
)

type shortVariants struct {
	User  string
	Agent string
}

const (
	annotationBaseHidden = "adaf.base_hidden"
	annotationBaseShort  = "adaf.base_short"
	annotationBaseLong   = "adaf.base_long"
)

var commandAudienceByPath = map[string]commandAudience{
	"init":            commandAudienceUserOnly,
	"run":             commandAudienceUserOnly,
	"web":             commandAudienceUserOnly,
	"attach":          commandAudienceUserOnly,
	"sessions":        commandAudienceUserOnly,
	"config":          commandAudienceUserOnly,
	"config agents":   commandAudienceUserOnly,
	"config pushover": commandAudienceUserOnly,
	"cleanup":         commandAudienceUserOnly,
	"stats":           commandAudienceUserOnly,
	"loop list":       commandAudienceUserOnly,
	"loop start":      commandAudienceUserOnly,

	"spawn":                commandAudienceAgentOnly,
	"spawn-status":         commandAudienceAgentOnly,
	"spawn-wait":           commandAudienceAgentOnly,
	"spawn-diff":           commandAudienceAgentOnly,
	"spawn-merge":          commandAudienceAgentOnly,
	"spawn-reject":         commandAudienceAgentOnly,
	"spawn-watch":          commandAudienceAgentOnly,
	"spawn-inspect":        commandAudienceAgentOnly,
	"spawn-reply":          commandAudienceAgentOnly,
	"parent-ask":           commandAudienceAgentOnly,
	"wait-for-spawns":      commandAudienceAgentOnly,
	"loop stop":            commandAudienceAgentOnly,
	"loop message":         commandAudienceAgentOnly,
	"loop call-supervisor": commandAudienceAgentOnly,
	"loop notify":          commandAudienceAgentOnly,
}

var commandShortByPath = map[string]shortVariants{
	"": {
		Agent: "ADAF agent command interface",
	},
	"status": {
		Agent: "Show project status and current execution context",
	},
	"turn": {
		Agent: "Read and write turn handoff logs",
	},
	"loop": {
		Agent: "Manage loop status and in-turn loop controls",
	},
}

func currentCLIRuntimeView() cliRuntimeView {
	if isAgentRuntimeContext() {
		return cliRuntimeViewAgent
	}
	return cliRuntimeViewUser
}

func isAgentRuntimeContext() bool {
	return session.IsAgentContext()
}

func enforceRuntimeCommandAccess(cmd *cobra.Command) error {
	return enforceCommandAccessForView(cmd, currentCLIRuntimeView())
}

func enforceCommandAccessForView(cmd *cobra.Command, view cliRuntimeView) error {
	path := commandPathFromLeaf(cmd)
	if path == "" {
		return nil
	}
	if view == cliRuntimeViewAgent && isSpawnedSubAgentRuntimeContext() && isTurnCommandPath(path) {
		return fmt.Errorf("%s is not available inside a spawned sub-agent context: spawned sub-agents cannot manage turns", cmd.CommandPath())
	}

	audience := audienceForPath(path)
	switch {
	case view == cliRuntimeViewAgent && audience == commandAudienceUserOnly:
		return fmt.Errorf("%s is not available inside an agent context", cmd.CommandPath())
	case view == cliRuntimeViewUser && audience == commandAudienceAgentOnly:
		return fmt.Errorf("%s is only available inside an agent context", cmd.CommandPath())
	default:
		if view == cliRuntimeViewAgent {
			if err := enforceLoopRoleCommandAccess(path, cmd.CommandPath()); err != nil {
				return err
			}
		}
		return nil
	}
}

func configureRuntimeCommandView(root *cobra.Command) {
	applyRuntimeView(root, currentCLIRuntimeView())
}

func applyRuntimeView(root *cobra.Command, view cliRuntimeView) {
	applyRuntimeViewRecursive(root, view)
}

func applyRuntimeViewRecursive(cmd *cobra.Command, view cliRuntimeView) {
	path := commandPathFromLeaf(cmd)

	rememberBaseState(cmd)
	applyVisibilityForView(cmd, path, view)
	applyShortForView(cmd, path, view)
	applyLongForView(cmd, path, view)

	for _, child := range cmd.Commands() {
		applyRuntimeViewRecursive(child, view)
	}
}

func commandPathFromLeaf(cmd *cobra.Command) string {
	if cmd == nil || cmd.Parent() == nil {
		return ""
	}

	var reversed []string
	for cur := cmd; cur != nil && cur.Parent() != nil; cur = cur.Parent() {
		reversed = append(reversed, strings.TrimSpace(cur.Name()))
	}

	parts := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		if reversed[i] == "" {
			continue
		}
		parts = append(parts, reversed[i])
	}
	return strings.Join(parts, " ")
}

func rememberBaseState(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}

	if _, ok := cmd.Annotations[annotationBaseHidden]; !ok {
		cmd.Annotations[annotationBaseHidden] = strconv.FormatBool(cmd.Hidden)
	}
	if _, ok := cmd.Annotations[annotationBaseShort]; !ok {
		cmd.Annotations[annotationBaseShort] = cmd.Short
	}
	if _, ok := cmd.Annotations[annotationBaseLong]; !ok {
		cmd.Annotations[annotationBaseLong] = cmd.Long
	}
}

func applyVisibilityForView(cmd *cobra.Command, path string, view cliRuntimeView) {
	baseHidden := false
	if raw, ok := cmd.Annotations[annotationBaseHidden]; ok {
		parsed, err := strconv.ParseBool(raw)
		if err == nil {
			baseHidden = parsed
		}
	}

	cmd.Hidden = baseHidden || !isVisibleInView(path, view)
}

func isVisibleInView(path string, view cliRuntimeView) bool {
	if path == "" {
		return true
	}
	if view == cliRuntimeViewAgent && isSpawnedSubAgentRuntimeContext() && isTurnCommandPath(path) {
		return false
	}

	audience := audienceForPath(path)
	switch view {
	case cliRuntimeViewAgent:
		if audience == commandAudienceUserOnly {
			return false
		}
		allowed, _ := loopRoleCommandAllowed(path)
		return allowed
	default:
		return audience != commandAudienceAgentOnly
	}
}

func audienceForPath(path string) commandAudience {
	current := strings.TrimSpace(path)
	for current != "" {
		if audience, ok := commandAudienceByPath[current]; ok {
			return audience
		}
		idx := strings.LastIndex(current, " ")
		if idx < 0 {
			break
		}
		current = strings.TrimSpace(current[:idx])
	}
	return commandAudienceBoth
}

func applyShortForView(cmd *cobra.Command, path string, view cliRuntimeView) {
	short := cmd.Annotations[annotationBaseShort]

	if variants, ok := commandShortByPath[path]; ok {
		if view == cliRuntimeViewAgent && strings.TrimSpace(variants.Agent) != "" {
			short = variants.Agent
		}
		if view == cliRuntimeViewUser && strings.TrimSpace(variants.User) != "" {
			short = variants.User
		}
	}

	cmd.Short = short
}

func applyLongForView(cmd *cobra.Command, path string, view cliRuntimeView) {
	longText := cmd.Annotations[annotationBaseLong]

	if path == "" && view == cliRuntimeViewAgent {
		longText = rootAgentLong()
	}

	cmd.Long = longText
}

func isTurnCommandPath(path string) bool {
	path = strings.TrimSpace(path)
	return path == "turn" || strings.HasPrefix(path, "turn ")
}

func isSpawnedSubAgentRuntimeContext() bool {
	return strings.TrimSpace(os.Getenv("ADAF_PARENT_TURN")) != ""
}

func enforceLoopRoleCommandAccess(path, commandPath string) error {
	allowed, reason := loopRoleCommandAllowed(path)
	if allowed {
		return nil
	}
	if strings.TrimSpace(reason) == "" {
		return nil
	}
	return fmt.Errorf("%s: %s", commandPath, reason)
}

func loopRoleCommandAllowed(path string) (bool, string) {
	path = strings.TrimSpace(path)
	if path != "loop stop" && path != "loop message" && path != "loop call-supervisor" {
		return true, ""
	}
	if strings.TrimSpace(os.Getenv("ADAF_LOOP_RUN_ID")) == "" {
		// Outside active loop-step contexts, keep command visible and allow the
		// command implementation to report missing runtime env as needed.
		return true, ""
	}
	pos := strings.ToLower(strings.TrimSpace(os.Getenv("ADAF_POSITION")))
	if !config.ValidPosition(pos) {
		return false, "ADAF_POSITION not set or invalid for this loop step"
	}
	switch path {
	case "loop stop":
		if config.PositionCanStopLoop(pos) {
			return true, ""
		}
		return false, "loop stop is supervisor-only"
	case "loop message":
		if config.PositionCanMessageLoop(pos) {
			return true, ""
		}
		if config.PositionCanCallSupervisor(pos) {
			if hasSupervisor, _ := currentLoopHasSupervisor(); hasSupervisor {
				return false, "loop message is supervisor-only; use `adaf loop call-supervisor \"...\"`"
			}
			return false, "loop message is supervisor-only"
		}
		return false, "loop message is supervisor-only"
	case "loop call-supervisor":
		if !config.PositionCanCallSupervisor(pos) {
			return false, "loop call-supervisor is manager-only"
		}
		hasSupervisor, reason := currentLoopHasSupervisor()
		if hasSupervisor {
			return true, ""
		}
		if strings.TrimSpace(reason) == "" {
			return false, "loop call-supervisor is unavailable: this loop has no supervisor step"
		}
		if reason == "this loop has no supervisor step" {
			return false, "loop call-supervisor is unavailable: this loop has no supervisor step"
		}
		return false, fmt.Sprintf("loop call-supervisor is unavailable: %s", reason)
	default:
		return true, ""
	}
}

func rootAgentLong() string {
	return fmt.Sprintf(`ADAF Agent Command Interface v%s

You are running inside an adaf-managed agent turn.
Only commands relevant to in-turn execution are shown.

Use:
  adaf --help
  adaf <command> --help`, buildinfo.Current().Version)
}
