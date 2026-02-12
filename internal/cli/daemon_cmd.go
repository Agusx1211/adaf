package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
)

var daemonCmd = &cobra.Command{
	Use:    "_session-daemon",
	Short:  "Internal: run a session daemon (do not call directly)",
	Hidden: true,
	RunE:   runDaemon,
}

func init() {
	daemonCmd.Flags().Int("id", 0, "Session ID")
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	sessionID, _ := cmd.Flags().GetInt("id")
	if sessionID == 0 {
		return fmt.Errorf("--id is required")
	}

	return session.RunDaemon(sessionID)
}
