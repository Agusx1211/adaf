package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var repairCmd = &cobra.Command{
	Use:     "repair",
	Aliases: []string{"fix"},
	Short:   "Repair missing .adaf project metadata directories",
	Long: `Repair and backfill project metadata directories under .adaf.
This is useful when older projects are missing required folders.`,
	RunE: runRepair,
}

func init() {
	rootCmd.AddCommand(repairCmd)
}

func runRepair(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return err
	}
	if !s.Exists() {
		return fmt.Errorf("no adaf project found (run 'adaf init' first)")
	}

	created, err := s.Repair()
	if err != nil {
		return fmt.Errorf("repairing project store: %w", err)
	}

	projectPath := filepath.Dir(s.Root())

	fmt.Println()
	fmt.Println(styleBoldCyan + "  Project Repair" + colorReset)
	fmt.Println(colorDim + "  ----------------------------------------" + colorReset)
	printField("Path", projectPath)

	if len(created) == 0 {
		fmt.Printf("  %sNo repairs needed.%s\n\n", styleBoldGreen, colorReset)
		return nil
	}

	fmt.Printf("  %sRecreated %d missing director", styleBoldGreen, len(created))
	if len(created) == 1 {
		fmt.Printf("y%s\n", colorReset)
	} else {
		fmt.Printf("ies%s\n", colorReset)
	}
	for _, dir := range created {
		fmt.Printf("  - %s\n", dir)
	}
	fmt.Println()
	return nil
}
