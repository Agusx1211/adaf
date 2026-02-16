package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "List all available skills",
	Long:  "List all available skill IDs with a one-line summary.",
	RunE:  runSkills,
}

var skillCmd = &cobra.Command{
	Use:   "skill <name>",
	Short: "Show full documentation for a skill",
	Long: `Show the full documentation for a named skill.

For the delegation skill, also includes available profiles from the
ADAF_DELEGATION_JSON environment variable when present.`,
	Args: cobra.ExactArgs(1),
	RunE: runSkill,
}

func init() {
	rootCmd.AddCommand(skillsCmd)
	rootCmd.AddCommand(skillCmd)
}

func runSkills(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	config.EnsureDefaultSkillCatalog(cfg)

	if len(cfg.Skills) == 0 {
		fmt.Println("No skills defined.")
		return nil
	}

	fmt.Println(styleBoldCyan + "Available Skills:" + colorReset)
	fmt.Println()
	for _, sk := range cfg.Skills {
		summary := firstSentence(sk.Short)
		fmt.Printf("  %s%-20s%s %s\n", styleBoldWhite, sk.ID, colorReset, summary)
	}
	fmt.Println()
	fmt.Printf("Use %sadaf skill <name>%s for full documentation.\n", styleBoldWhite, colorReset)
	return nil
}

func runSkill(cmd *cobra.Command, args []string) error {
	skillID := strings.ToLower(strings.TrimSpace(args[0]))

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sk := cfg.FindSkill(skillID)
	if sk == nil {
		return fmt.Errorf("skill %q not found", skillID)
	}

	// Print the long documentation, falling back to short if long is empty.
	doc := sk.Long
	if doc == "" {
		doc = sk.Short
	}
	fmt.Println(doc)

	// For the delegation skill, append available profiles from env.
	if sk.ID == config.SkillDelegation {
		delegJSON := os.Getenv("ADAF_DELEGATION_JSON")
		if delegJSON != "" {
			var deleg config.DelegationConfig
			if err := json.Unmarshal([]byte(delegJSON), &deleg); err == nil && len(deleg.Profiles) > 0 {
				fmt.Print("\n## Available Profiles (from current context)\n\n")
				for _, dp := range deleg.Profiles {
					roles, _ := dp.EffectiveRoles()
					line := fmt.Sprintf("- **%s**", dp.Name)
					if len(roles) == 1 {
						line += fmt.Sprintf(" (role=%s)", roles[0])
					} else if len(roles) > 1 {
						line += fmt.Sprintf(" (roles=%s)", strings.Join(roles, "/"))
					}
					fmt.Println(line)
				}
			}
		}
	}

	return nil
}

// firstSentence returns the first sentence (up to first period + space, or newline).
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, ". "); idx >= 0 {
		return s[:idx+1]
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	// Cap at reasonable length.
	if len(s) > 100 {
		return s[:100] + "..."
	}
	return s
}
