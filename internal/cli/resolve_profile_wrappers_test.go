package cli

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
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

func TestResolvePMProfile_UsesPMPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{})

	cmd := newResolvePMProfileCmd(t)

	prof, globalCfg, cmdOverride, err := resolvePMProfile(cmd)
	if err != nil {
		t.Fatalf("resolvePMProfile() error = %v", err)
	}
	if prof.Name != "pm:claude" {
		t.Fatalf("profile name = %q, want %q", prof.Name, "pm:claude")
	}
	if globalCfg == nil {
		t.Fatal("global config is nil")
	}
	if cmdOverride != "" {
		t.Fatalf("command override = %q, want empty", cmdOverride)
	}
}

func TestResolvePMProfile_ReturnsPopulatedGlobalConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "pm-template", "agent": "claude"},
		},
	})

	cmd := newResolvePMProfileCmd(t)

	prof, globalCfg, _, err := resolvePMProfile(cmd)
	if err != nil {
		t.Fatalf("resolvePMProfile() error = %v", err)
	}
	if globalCfg == nil {
		t.Fatal("global config is nil")
	}
	if prof == nil {
		t.Fatal("profile is nil")
	}
	if got := globalCfg.FindProfile("pm-template"); got == nil {
		t.Fatal("global config did not retain profiles from config file")
	}
}

func TestResolvePMProfile_CustomRoleUsedInPMPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const customManagerTitle = "CUSTOM PROJECT MANAGER"

	writeGlobalConfig(t, home, map[string]any{
		"roles": []map[string]any{
			{
				"name":  config.RoleManager,
				"title": customManagerTitle,
			},
		},
	})

	cmd := newResolvePMProfileCmd(t)
	prof, globalCfg, _, err := resolvePMProfile(cmd)
	if err != nil {
		t.Fatalf("resolvePMProfile() error = %v", err)
	}

	storeDir := t.TempDir()
	s, err := store.New(storeDir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	projCfg := &store.ProjectConfig{Name: "pm-prompt-test", RepoPath: storeDir}
	if err := s.Init(*projCfg); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	project, err := s.LoadProject()
	if err != nil {
		t.Fatalf("store.LoadProject: %v", err)
	}

	prompt, err := buildPMPrompt(s, project, "", prof, globalCfg, "status blockers")
	if err != nil {
		t.Fatalf("buildPMPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "# Your Role: "+customManagerTitle) {
		t.Fatalf("prompt missing custom manager title = %q", customManagerTitle)
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

func newResolvePMProfileCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("agent", "claude", "")
	cmd.Flags().String("model", "", "")
	return cmd
}
