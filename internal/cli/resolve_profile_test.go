package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveProfile_ProfileFlagLooksUpByName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude", "model": "opus-4"},
		},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "my-claude")

	prof, globalCfg, cmdOverride, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if prof.Name != "my-claude" {
		t.Fatalf("profile name = %q, want %q", prof.Name, "my-claude")
	}
	if prof.Agent != "claude" {
		t.Fatalf("profile agent = %q, want %q", prof.Agent, "claude")
	}
	if prof.Model != "opus-4" {
		t.Fatalf("profile model = %q, want %q", prof.Model, "opus-4")
	}
	if globalCfg == nil {
		t.Fatal("globalCfg is nil")
	}
	if cmdOverride != "" {
		t.Fatalf("cmdOverride = %q, want empty", cmdOverride)
	}
}

func TestResolveProfile_ProfileNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "nonexistent")

	_, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err == nil {
		t.Fatal("expected error for nonexistent profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, want 'not found'", err.Error())
	}
}

func TestResolveProfile_ProfileWithModelOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude", "model": "sonnet-4"},
		},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "my-claude")
	_ = cmd.Flags().Set("model", "opus-4")

	prof, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if prof.Model != "opus-4" {
		t.Fatalf("model = %q, want %q (--model should override profile)", prof.Model, "opus-4")
	}
}

func TestResolveProfile_ProfileWithReasoningLevel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude"},
		},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "my-claude")

	prof, _, _, err := resolveProfile(cmd, ProfileResolveOpts{
		Prefix:         "ask",
		ReasoningLevel: "high",
	})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if prof.ReasoningLevel != "high" {
		t.Fatalf("reasoning_level = %q, want %q", prof.ReasoningLevel, "high")
	}
}

func TestResolveProfile_AgentFlagBuildsSyntheticProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{})

	cmd := newResolveProfileCmd(t)
	// Default agent is "claude" (no --profile set, so it falls through to --agent).

	prof, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if prof.Name != "ask:claude" {
		t.Fatalf("synthetic profile name = %q, want %q", prof.Name, "ask:claude")
	}
	if prof.Agent != "claude" {
		t.Fatalf("agent = %q, want %q", prof.Agent, "claude")
	}
}

func TestResolveProfile_AgentFlagWithModelOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("model", "custom-model-v2")

	prof, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "pm"})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if prof.Model != "custom-model-v2" {
		t.Fatalf("model = %q, want %q", prof.Model, "custom-model-v2")
	}
}

func TestResolveProfile_UnknownAgentReturnsError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("agent", "nonexistent-agent")

	_, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("error = %q, want 'unknown agent'", err.Error())
	}
}

func TestResolveProfile_ProfileCmdOverrideFromAgentsConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude"},
		},
	})
	writeAgentsConfig(t, home, map[string]any{
		"agents": map[string]any{
			"claude": map[string]any{
				"name": "claude",
				"path": "/custom/bin/claude",
			},
		},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "my-claude")

	_, _, cmdOverride, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if cmdOverride != "/custom/bin/claude" {
		t.Fatalf("cmdOverride = %q, want %q", cmdOverride, "/custom/bin/claude")
	}
}

func TestResolveProfile_CustomCmdOverridesAgentsConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude"},
		},
	})
	writeAgentsConfig(t, home, map[string]any{
		"agents": map[string]any{
			"claude": map[string]any{
				"name": "claude",
				"path": "/agents-config/claude",
			},
		},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "my-claude")

	_, _, cmdOverride, err := resolveProfile(cmd, ProfileResolveOpts{
		Prefix:    "ask",
		CustomCmd: "/explicit/claude",
	})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if cmdOverride != "/explicit/claude" {
		t.Fatalf("cmdOverride = %q, want %q (--command should override agents config)", cmdOverride, "/explicit/claude")
	}
}

func TestResolveProfile_WhitespaceIsTrimmed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{
		"profiles": []map[string]any{
			{"name": "my-claude", "agent": "claude"},
		},
	})

	cmd := newResolveProfileCmd(t)
	_ = cmd.Flags().Set("profile", "  my-claude  ")

	prof, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: "ask"})
	if err != nil {
		t.Fatalf("resolveProfile() error = %v", err)
	}
	if prof.Name != "my-claude" {
		t.Fatalf("profile name = %q, want %q", prof.Name, "my-claude")
	}
}

func TestResolveProfile_PrefixUsedInSyntheticName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeGlobalConfig(t, home, map[string]any{})

	tests := []struct {
		prefix string
		want   string
	}{
		{prefix: "ask", want: "ask:claude"},
		{prefix: "pm", want: "pm:claude"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			cmd := newResolveProfileCmd(t)
			prof, _, _, err := resolveProfile(cmd, ProfileResolveOpts{Prefix: tt.prefix})
			if err != nil {
				t.Fatalf("resolveProfile() error = %v", err)
			}
			if prof.Name != tt.want {
				t.Fatalf("name = %q, want %q", prof.Name, tt.want)
			}
		})
	}
}

// --- helpers ---

func newResolveProfileCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("agent", "claude", "")
	cmd.Flags().String("model", "", "")
	return cmd
}

func writeGlobalConfig(t *testing.T, home string, cfg map[string]any) {
	t.Helper()
	dir := filepath.Join(home, ".adaf")
	os.MkdirAll(dir, 0755)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal global config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0644); err != nil {
		t.Fatalf("write global config: %v", err)
	}
}

func writeAgentsConfig(t *testing.T, home string, cfg map[string]any) {
	t.Helper()
	dir := filepath.Join(home, ".adaf")
	os.MkdirAll(dir, 0755)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal agents config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agents.json"), data, 0644); err != nil {
		t.Fatalf("write agents config: %v", err)
	}
}
