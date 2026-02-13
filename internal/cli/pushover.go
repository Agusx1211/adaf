package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/pushover"
)

var pushoverCmd = &cobra.Command{
	Use:   "pushover",
	Short: "Manage Pushover notification settings",
	Long:  "Configure Pushover credentials so loop agents can send push notifications.",
}

var pushoverSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure Pushover credentials",
	Long: `Set up Pushover integration by providing your User Key and Application Token.

You can find these at https://pushover.net:
  - User Key: shown on your Pushover dashboard
  - App Token: create an application at https://pushover.net/apps/build`,
	RunE: pushoverSetup,
}

var pushoverTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a test Pushover notification",
	RunE:  pushoverTest,
}

var pushoverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Pushover configuration status",
	RunE:  pushoverStatusFn,
}

func init() {
	pushoverSetupCmd.Flags().String("user-key", "", "Pushover user key")
	pushoverSetupCmd.Flags().String("app-token", "", "Pushover application token")
	pushoverCmd.AddCommand(pushoverSetupCmd, pushoverTestCmd, pushoverStatusCmd)
	configCmd.AddCommand(pushoverCmd)
}

func pushoverSetup(cmd *cobra.Command, args []string) error {
	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	userKey, _ := cmd.Flags().GetString("user-key")
	appToken, _ := cmd.Flags().GetString("app-token")

	reader := bufio.NewReader(os.Stdin)

	if userKey == "" {
		current := globalCfg.Pushover.UserKey
		prompt := "  Pushover User Key"
		if current != "" {
			prompt += fmt.Sprintf(" [%s]", maskSecret(current))
		}
		prompt += ": "
		fmt.Print(prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			userKey = input
		} else {
			userKey = current
		}
	}

	if appToken == "" {
		current := globalCfg.Pushover.AppToken
		prompt := "  Pushover App Token"
		if current != "" {
			prompt += fmt.Sprintf(" [%s]", maskSecret(current))
		}
		prompt += ": "
		fmt.Print(prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			appToken = input
		} else {
			appToken = current
		}
	}

	if userKey == "" || appToken == "" {
		return fmt.Errorf("both user key and app token are required")
	}

	globalCfg.Pushover.UserKey = userKey
	globalCfg.Pushover.AppToken = appToken

	if err := config.Save(globalCfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\n  %sPushover credentials saved to ~/.adaf/config.json%s\n", styleBoldGreen, colorReset)
	return nil
}

func pushoverTest(cmd *cobra.Command, args []string) error {
	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if !pushover.Configured(&globalCfg.Pushover) {
		return fmt.Errorf("pushover not configured: run 'adaf config pushover setup' first")
	}

	msg := pushover.Message{
		Title:    "adaf test",
		Body:     "This is a test notification from adaf.",
		Priority: pushover.PriorityNormal,
	}

	fmt.Print("  Sending test notification... ")
	if err := pushover.Send(&globalCfg.Pushover, msg); err != nil {
		fmt.Println()
		return fmt.Errorf("test failed: %w", err)
	}

	fmt.Printf("%sOK%s\n", styleBoldGreen, colorReset)
	return nil
}

func pushoverStatusFn(cmd *cobra.Command, args []string) error {
	globalCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	printHeader("Pushover")
	if pushover.Configured(&globalCfg.Pushover) {
		printField("User Key", maskSecret(globalCfg.Pushover.UserKey))
		printField("App Token", maskSecret(globalCfg.Pushover.AppToken))
		printFieldColored("Status", "configured", colorGreen)
	} else {
		printFieldColored("Status", "not configured", colorYellow)
		fmt.Println()
		fmt.Printf("  Run %sadaf config pushover setup%s to configure.\n", styleBoldWhite, colorReset)
	}
	return nil
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:4] + strings.Repeat("*", len(secret)-4)
}
