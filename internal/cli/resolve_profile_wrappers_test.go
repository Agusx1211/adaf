package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveAskProfile_UsesAskSpecificFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude"},
		},
	})

	cmd := newResolveAskProfileCmd(t)
	_ = cmd.Flags().Set("profile", "my-claude")
	_ = cmd.Flags().Set("command", "/explicit/claude")
	_ = cmd.Flags().Set("reasoning-level", "xhigh")

	prof, cmdOverride, err := resolveAskProfile(cmd)
	if err != nil {
		t.Fatalf("resolveAskProfile() error = %v", err)
	}
	if prof.ReasoningLevel != "xhigh" {
		t.Fatalf("reasoning level = %q, want %q", prof.ReasoningLevel, "xhigh")
	}
	if cmdOverride != "/explicit/claude" {
		t.Fatalf("command override = %q, want %q", cmdOverride, "/explicit/claude")
	}
}

func newResolveAskProfileCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("agent", "claude", "")
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("command", "", "")
	cmd.Flags().String("reasoning-level", "", "")
	return cmd
}
