package cli

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/buildinfo"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/tui"
)

const (
	// ANSI color codes
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"

	// Combined styles
	styleBoldCyan   = "\033[1;36m"
	styleBoldGreen  = "\033[1;32m"
	styleBoldYellow = "\033[1;33m"
	styleBoldRed    = "\033[1;31m"
	styleBoldBlue   = "\033[1;34m"
	styleBoldWhite  = "\033[1;37m"
)

var rootCmd = &cobra.Command{
	Use:   "adaf",
	Short: "Autonomous Developer Agent Flow",
	Long: colorBold + `
     _       _        __
    / \   __| | __ _ / _|
   / _ \ / _` + "`" + ` |/ _` + "`" + ` | |_
  / ___ \ (_| | (_| |  _|
 /_/   \_\__,_|\__,_|_|` + colorReset + `

  ` + styleBoldCyan + `Autonomous Developer Agent Flow` + colorReset + ` v` + buildinfo.Current().Version + `

  Orchestrate AI agents to build, plan, and maintain software projects.
  adaf tracks plans, issues, turn logs, and recordings
  so that multiple AI agents (and humans) can collaborate effectively.

  Run ` + styleBoldWhite + `adaf status` + colorReset + ` for project overview, or ` + styleBoldWhite + `adaf init` + colorReset + ` to start a new project.

` + colorBold + `Getting Started:` + colorReset + `
  adaf init --name my-project     Initialize a project
  adaf plan create --id core --title "Core"
  adaf plan switch core           Select active plan
  adaf run --agent claude         Run an agent session
  adaf status                     Show project overview
  adaf                            Launch interactive TUI

` + colorBold + `Supported Agents:` + colorReset + `
  claude, codex, vibe, opencode, gemini, generic

` + colorBold + `More Info:` + colorReset + `
  https://github.com/agusx1211/adaf`,

	RunE: func(cmd *cobra.Command, args []string) error {
		if isAgentRuntimeContext() {
			return cmd.Help()
		}

		// When run with no subcommand, show a brief status or help
		s, err := openStore()
		if err != nil {
			// No project found, show help
			return cmd.Help()
		}
		if !s.Exists() {
			fmt.Println(styleBoldYellow + "No adaf project found in this directory." + colorReset)
			fmt.Println("Run " + styleBoldWhite + "adaf init" + colorReset + " to create one.")
			return nil
		}
		// If running in a terminal, launch the unified TUI.
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return tui.RunApp(s)
		}
		// Non-interactive: show brief status.
		return runStatusBrief(s)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.PersistentFlags().Bool("debug", false, "Enable verbose debug logging to ~/.adaf/debug/")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := enforceRuntimeCommandAccess(cmd); err != nil {
			return err
		}

		debugFlag, _ := cmd.Flags().GetBool("debug")
		if !debugFlag && !debug.ShouldEnableFromEnv() {
			return nil
		}
		logPath, err := debug.Init()
		if err != nil {
			return fmt.Errorf("initializing debug logger: %w", err)
		}
		fmt.Fprintf(os.Stderr, "%s[debug]%s logging to %s\n", colorDim, colorReset, logPath)
		bi := buildinfo.Current()
		debug.LogKV("cli", "adaf starting",
			"version", bi.Version,
			"commit", bi.CommitHash,
			"build_date", bi.BuildDate,
			"pid", os.Getpid(),
			"command", cmd.Name(),
			"args", args,
		)
		return nil
	}
}

// Execute runs the root command.
func Execute() {
	defer debug.Close()
	configureRuntimeCommandView(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		debug.Logf("cli", "exit with error: %v", err)
		fmt.Fprintf(os.Stderr, "%sError: %s%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
	debug.Log("cli", "exit success")
}
