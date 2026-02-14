package cli

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
	"github.com/spf13/cobra"
)

func TestPMCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "pm [message]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("pm command not registered on rootCmd")
	}
}

func TestPMCmdFlags(t *testing.T) {
	tests := []struct {
		name        string
		defaultWant string
	}{
		{name: "agent", defaultWant: "claude"},
		{name: "profile", defaultWant: ""},
		{name: "model", defaultWant: ""},
		{name: "plan", defaultWant: ""},
		{name: "session", defaultWant: "false"},
	}

	for _, tt := range tests {
		flag := pmCmd.Flags().Lookup(tt.name)
		if flag == nil {
			t.Fatalf("pm --%s flag missing", tt.name)
		}
		if flag.DefValue != tt.defaultWant {
			t.Fatalf("pm --%s default = %q, want %q", tt.name, flag.DefValue, tt.defaultWant)
		}
	}

	sessionFlag := pmCmd.Flags().Lookup("session")
	if sessionFlag == nil {
		t.Fatal("pm --session flag missing")
	}
	if sessionFlag.Shorthand != "s" {
		t.Fatalf("pm --session shorthand = %q, want %q", sessionFlag.Shorthand, "s")
	}
}

func TestPMRejectsAgentContext(t *testing.T) {
	t.Setenv("ADAF_AGENT", "1")
	cmd := &cobra.Command{}
	cmd.Flags().String("agent", "claude", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("plan", "", "")
	cmd.Flags().BoolP("session", "s", false, "")

	err := runPM(cmd, []string{"review current plan"})
	if err == nil {
		t.Fatal("runPM() error = nil, want agent-context rejection")
	}
	if !strings.Contains(err.Error(), "not available inside an agent context") {
		t.Fatalf("runPM() error = %q, want context rejection message", err.Error())
	}
}

func TestBuildPMPrompt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	projCfg := &store.ProjectConfig{
		Name:     "demo-project",
		RepoPath: t.TempDir(),
	}
	prof := &config.Profile{
		Name:  "pm:claude",
		Agent: "claude",
	}
	userMessage := "Please prioritize blockers and update the plan."

	globalCfg := &config.GlobalConfig{}
	got, err := buildPMPrompt(s, projCfg, "", prof, globalCfg, userMessage)
	if err != nil {
		t.Fatalf("buildPMPrompt() error: %v", err)
	}

	wants := []string{
		"# Role: Project Manager",
		"## Your Capabilities",
		"## User Message",
		userMessage,
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}
