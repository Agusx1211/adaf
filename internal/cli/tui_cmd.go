package cli

import (
	"github.com/agusx1211/adaf/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive terminal UI",
	Long:  `Opens a full-screen interactive dashboard for managing the project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStoreRequired()
		if err != nil {
			return err
		}
		return tui.Run(s)
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
