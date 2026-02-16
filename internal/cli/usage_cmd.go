package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/usage"
	"github.com/spf13/cobra"
)

var usageCmd = &cobra.Command{
	Use:     "usage",
	Aliases: []string{"limits", "quota"},
	Short:   "Show API usage limits for configured agents",
	Long: `Display current usage limits and quotas for Claude, Codex, and Gemini.

This command queries each provider's usage API to show how much of your
rate limit has been consumed and when it will reset.

Examples:
  adaf usage              # Show usage for all available providers
  adaf usage --provider claude   # Show only Claude usage
  adaf usage --json       # Output as JSON`,
	RunE: runUsage,
}

func init() {
	usageCmd.Flags().String("provider", "", "Provider to query (claude, codex, gemini)")
	usageCmd.Flags().Bool("json", false, "Output as JSON")
	usageCmd.Flags().Duration("timeout", 10*time.Second, "Timeout for fetching usage")
	rootCmd.AddCommand(usageCmd)
}

func runUsage(cmd *cobra.Command, args []string) error {
	providerFlag, _ := cmd.Flags().GetString("provider")
	asJSON, _ := cmd.Flags().GetBool("json")
	timeout, _ := cmd.Flags().GetDuration("timeout")

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	providers := usage.DefaultProviders()

	if providerFlag != "" {
		providers = filterProviders(providers, providerFlag)
	}

	var snapshots []usage.UsageSnapshot
	var errs []error

	for _, p := range providers {
		if !p.HasCredentials() {
			continue
		}

		snapshot, err := p.FetchUsage(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		snapshots = append(snapshots, snapshot)
	}

	if asJSON {
		return printUsageJSON(snapshots, errs)
	}

	return printUsageHuman(snapshots, errs)
}

func filterProviders(providers []usage.Provider, name string) []usage.Provider {
	name = strings.ToLower(strings.TrimSpace(name))
	var filtered []usage.Provider
	for _, p := range providers {
		if strings.ToLower(p.Name().String()) == name {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func printUsageHuman(snapshots []usage.UsageSnapshot, errors []error) error {
	if len(snapshots) == 0 && len(errors) == 0 {
		fmt.Printf("\n  %sNo providers configured.%s\n\n", colorDim, colorReset)
		fmt.Printf("  Configure Claude: %s~/.claude/.credentials.json%s\n", colorDim, colorReset)
		fmt.Printf("  Configure Codex:  %s~/.codex/auth.json%s (with auth_mode: chatgpt)\n\n", colorDim, colorReset)
		return nil
	}

	for _, snap := range snapshots {
		if len(snap.Limits) > 0 {
			printUsageSnapshot(snap)
		}
	}

	for _, err := range errors {
		fmt.Printf("\n  %sError:%s %s\n", colorRed, colorReset, err.Error())
	}

	fmt.Println()
	return nil
}

func printUsageSnapshot(snap usage.UsageSnapshot) {
	fmt.Printf("\n%s%s%s\n", styleBoldCyan, snap.Provider.DisplayName(), colorReset)
	fmt.Println(colorDim + strings.Repeat("-", len(snap.Provider.DisplayName())+2) + colorReset)

	if len(snap.Limits) == 0 {
		fmt.Printf("  %sNo limits data available%s\n", colorDim, colorReset)
		return
	}

	for _, limit := range snap.Limits {
		level := limit.Level(70, 90)
		limitColor := usageLevelColor(level)

		barWidth := 20
		filled := int(limit.UtilizationPct / 100 * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled

		bar := limitColor + strings.Repeat("█", filled) + colorDim + strings.Repeat("░", empty) + colorReset

		resetText := ""
		if limit.ResetsAt != nil {
			resetText = formatResetTime(*limit.ResetsAt)
		}

		fmt.Printf("  %-16s [%s] %s%.0f%%%s%s\n",
			limit.Name,
			bar,
			limitColor,
			limit.UtilizationPct,
			colorReset,
			resetText,
		)
	}
}

func usageLevelColor(level usage.UsageLevel) string {
	switch level {
	case usage.LevelNormal:
		return colorGreen
	case usage.LevelWarning:
		return colorYellow
	case usage.LevelCritical:
		return colorRed
	case usage.LevelExhausted:
		return colorRed
	default:
		return colorWhite
	}
}

func formatResetTime(t time.Time) string {
	now := time.Now()
	if t.Before(now) {
		return " (resets soon)"
	}

	d := t.Sub(now)
	switch {
	case d < time.Minute:
		return fmt.Sprintf(" (resets in %ds)", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf(" (resets in %dm)", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf(" (resets in %dh)", int(d.Hours()))
	default:
		return fmt.Sprintf(" (resets in %dd)", int(d.Hours()/24))
	}
}

func printUsageJSON(snapshots []usage.UsageSnapshot, errors []error) error {
	type limitJSON struct {
		Name           string  `json:"name"`
		UtilizationPct float64 `json:"utilization_pct"`
		RemainingPct   float64 `json:"remaining_pct"`
		ResetsAt       string  `json:"resets_at,omitempty"`
	}

	type snapshotJSON struct {
		Provider  string      `json:"provider"`
		Level     string      `json:"level"`
		Timestamp string      `json:"timestamp"`
		Limits    []limitJSON `json:"limits"`
	}

	type outputJSON struct {
		Snapshots []snapshotJSON `json:"snapshots"`
		Errors    []string       `json:"errors,omitempty"`
	}

	var out outputJSON

	for _, snap := range snapshots {
		var limits []limitJSON
		for _, l := range snap.Limits {
			resetsAt := ""
			if l.ResetsAt != nil {
				resetsAt = l.ResetsAt.Format(time.RFC3339)
			}
			limits = append(limits, limitJSON{
				Name:           l.Name,
				UtilizationPct: l.UtilizationPct,
				RemainingPct:   l.RemainingPct(),
				ResetsAt:       resetsAt,
			})
		}
		out.Snapshots = append(out.Snapshots, snapshotJSON{
			Provider:  snap.Provider.DisplayName(),
			Level:     snap.OverallLevel.String(),
			Timestamp: snap.Timestamp.Format(time.RFC3339),
			Limits:    limits,
		})
	}

	for _, err := range errors {
		out.Errors = append(out.Errors, err.Error())
	}

	fmt.Println(marshalJSON(out))
	return nil
}

func marshalJSON(v interface{}) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
