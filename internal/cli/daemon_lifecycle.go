package cli

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var daemonLifecycleCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the background web daemon",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the web daemon in the background",
	Args:  cobra.NoArgs,
	RunE:  runDaemonLifecycleStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the web daemon",
	Args:  cobra.NoArgs,
	RunE:  runDaemonLifecycleStop,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show web daemon status",
	Args:  cobra.NoArgs,
	RunE:  runDaemonLifecycleStatus,
}

func init() {
	addWebServerFlags(daemonStartCmd, "open-browser", "Open browser automatically", true)
	daemonStartCmd.Flags().Bool("open", false, "Open browser automatically")
	_ = daemonStartCmd.Flags().MarkHidden("open")
	_ = daemonStartCmd.Flags().MarkHidden("daemon")

	daemonLifecycleCmd.AddCommand(daemonStartCmd, daemonStopCmd, daemonStatusCmd)
}

func runDaemonLifecycleStart(cmd *cobra.Command, args []string) error {
	return runWebAsDaemonStart(cmd, args, false)
}

func runWebAsDaemonStart(cmd *cobra.Command, args []string, defaultOpenBrowser bool) error {
	openBrowser := defaultOpenBrowser
	if flag := cmd.Flags().Lookup("open-browser"); flag != nil {
		openBrowser, _ = cmd.Flags().GetBool("open-browser")
	}

	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil {
		return fmt.Errorf("checking web daemon status: %w", err)
	}
	if running {
		printDaemonStatusLine(cmd, state)
		return nil
	}

	if flag := cmd.Flags().Lookup("daemon"); flag != nil {
		_ = cmd.Flags().Set("daemon", "true")
	}
	if flag := cmd.Flags().Lookup("open"); flag != nil {
		_ = cmd.Flags().Set("open", strconv.FormatBool(openBrowser))
	}
	return runWebServe(cmd, args)
}

func runDaemonLifecycleStop(cmd *cobra.Command, args []string) error {
	return runWebStop(cmd, args)
}

func runDaemonLifecycleStatus(cmd *cobra.Command, args []string) error {
	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil {
		return fmt.Errorf("checking web daemon status: %w", err)
	}
	if !running {
		fmt.Fprintln(cmd.OutOrStdout(), "Web daemon not running.")
		return nil
	}
	printDaemonStatusLine(cmd, state)
	return nil
}

func printDaemonStatusLine(cmd *cobra.Command, state webRuntimeState) {
	url := daemonStatusURL(state)
	fmt.Fprintf(cmd.OutOrStdout(), "Web daemon running (PID %d)\n", state.PID)
	if url != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "URL: %s\n", url)
	}
}

func daemonStatusURL(state webRuntimeState) string {
	url := strings.TrimSpace(state.URL)
	if url != "" {
		return url
	}
	if state.Port <= 0 {
		return ""
	}
	scheme := strings.TrimSpace(state.Scheme)
	if scheme == "" {
		scheme = "http"
	}

	host := strings.TrimSpace(state.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	host = strings.TrimPrefix(host, ":")
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(state.Port)))
}
