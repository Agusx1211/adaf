package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
)

var sessionsStopCmd = &cobra.Command{
	Use:   "stop [session-id|profile]",
	Short: "Stop a running session",
	Long: `Stop one or more running sessions without opening interactive attach mode.

By default, this command stops one session resolved by session ID or profile match.
Use --all to stop every currently running session.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionsStop,
}

func init() {
	sessionsStopCmd.Flags().Bool("force", false, "Force kill (SIGTERM process group) if graceful cancel does not stop")
	sessionsStopCmd.Flags().Bool("all", false, "Stop all running sessions")
	sessionsCmd.AddCommand(sessionsStopCmd)
}

func runSessionsStop(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("session management is not available inside an agent context")
	}

	force, _ := cmd.Flags().GetBool("force")
	stopAll, _ := cmd.Flags().GetBool("all")
	out := cmd.OutOrStdout()

	if stopAll {
		if len(args) > 0 {
			return fmt.Errorf("session-id argument cannot be used with --all")
		}
		return stopAllSessions(out, force)
	}

	if len(args) == 0 {
		return fmt.Errorf("session-id is required unless --all is set")
	}

	meta, err := session.FindSessionByPartial(args[0])
	if err != nil {
		return fmt.Errorf("finding session: %w", err)
	}

	_, err = stopSession(meta, out, force)
	return err
}

func stopAllSessions(out io.Writer, force bool) error {
	sessions, err := session.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	active := make([]session.SessionMeta, 0, len(sessions))
	for i := range sessions {
		if session.IsActiveStatus(sessions[i].Status) {
			active = append(active, sessions[i])
		}
	}

	stopped := 0
	var stopErrs []string
	for i := range active {
		result, err := stopSession(&active[i], out, force)
		if err != nil {
			stopErrs = append(stopErrs, fmt.Sprintf("#%d: %v", active[i].ID, err))
			continue
		}
		if result.stopped {
			stopped++
		}
	}

	fmt.Fprintf(out, "Stopped %d sessions\n", stopped)
	if len(stopErrs) > 0 {
		return fmt.Errorf("stopping sessions failed: %s", strings.Join(stopErrs, "; "))
	}
	return nil
}

type sessionStopResult struct {
	stopped bool
}

func stopSession(meta *session.SessionMeta, out io.Writer, force bool) (sessionStopResult, error) {
	if meta == nil {
		return sessionStopResult{}, fmt.Errorf("session metadata is required")
	}

	if !session.IsActiveStatus(meta.Status) {
		fmt.Fprintf(out, "Session #%d: already finished (%s)\n", meta.ID, meta.Status)
		return sessionStopResult{}, nil
	}

	client, err := session.ConnectToSession(meta.ID)
	if err != nil {
		refreshed, loadErr := session.LoadMeta(meta.ID)
		if loadErr == nil && !session.IsActiveStatus(refreshed.Status) {
			fmt.Fprintf(out, "Session #%d: already finished (%s)\n", refreshed.ID, refreshed.Status)
			return sessionStopResult{}, nil
		}
		return sessionStopResult{}, fmt.Errorf("connecting to session %d: %w", meta.ID, err)
	}

	cancelErr := client.Cancel()
	_ = client.Close()

	if cancelErr != nil {
		refreshed, loadErr := session.LoadMeta(meta.ID)
		if loadErr == nil && !session.IsActiveStatus(refreshed.Status) {
			fmt.Fprintf(out, "Session #%d: already finished (%s)\n", refreshed.ID, refreshed.Status)
			return sessionStopResult{}, nil
		}
		return sessionStopResult{}, fmt.Errorf("sending cancel to session %d: %w", meta.ID, cancelErr)
	}

	fmt.Fprintf(out, "Session #%d stopped\n", meta.ID)

	if force {
		time.Sleep(2 * time.Second)
		if isPIDAlive(meta.PID) {
			if err := syscall.Kill(-meta.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
				return sessionStopResult{stopped: true}, fmt.Errorf("force-killing session %d: %w", meta.ID, err)
			}
			fmt.Fprintf(out, "Session #%d force-killed\n", meta.ID)
		}
	}

	return sessionStopResult{stopped: true}, nil
}

func isPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
