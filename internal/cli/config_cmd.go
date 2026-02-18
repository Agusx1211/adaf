package cli

import "github.com/spf13/cobra"

var configCmd = &cobra.Command{
	Use:     "config",
	Aliases: []string{"cfg"},
	Short:   "Manage adaf configuration",
	Long: `Manage adaf configuration and local tool integrations.

Use subcommands like:
  adaf config agents
  adaf config pushover`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	configCmd.SuggestFor = append(configCmd.SuggestFor, "profile", "profiles")
	rootCmd.AddCommand(configCmd)
}
