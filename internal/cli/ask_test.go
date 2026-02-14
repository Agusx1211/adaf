package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildAskLoopDefinition(t *testing.T) {
	loopDef, maxCycles := buildAskLoopDefinition("ask:claude", "fix the tests")

	if maxCycles != 1 {
		t.Fatalf("maxCycles = %d, want 1", maxCycles)
	}
	if loopDef.Name != "ask" {
		t.Fatalf("loop name = %q, want %q", loopDef.Name, "ask")
	}
	if len(loopDef.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(loopDef.Steps))
	}
	step := loopDef.Steps[0]
	if step.Profile != "ask:claude" {
		t.Fatalf("step profile = %q, want %q", step.Profile, "ask:claude")
	}
	if step.Turns != 1 {
		t.Fatalf("step turns = %d, want 1", step.Turns)
	}
	if step.Instructions != "fix the tests" {
		t.Fatalf("instructions = %q, want %q", step.Instructions, "fix the tests")
	}
}

func TestResolveAskPrompt_PositionalArg(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("prompt", "", "")
	prompt, err := resolveAskPrompt(cmd, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != "hello world" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello world")
	}
}

func TestResolveAskPrompt_FlagOverPositional(t *testing.T) {
	// Positional args take precedence over --prompt flag.
	cmd := &cobra.Command{}
	cmd.Flags().String("prompt", "", "")
	_ = cmd.Flags().Set("prompt", "from flag")
	prompt, err := resolveAskPrompt(cmd, []string{"from args"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != "from args" {
		t.Fatalf("prompt = %q, want %q", prompt, "from args")
	}
}

func TestResolveAskPrompt_FlagOnly(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("prompt", "", "")
	_ = cmd.Flags().Set("prompt", "from flag")
	prompt, err := resolveAskPrompt(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompt != "from flag" {
		t.Fatalf("prompt = %q, want %q", prompt, "from flag")
	}
}

func TestResolveAskPrompt_NoInput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("prompt", "", "")
	_, err := resolveAskPrompt(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no prompt provided")
	}
	if !strings.Contains(err.Error(), "no prompt provided") {
		t.Fatalf("error = %q, want 'no prompt provided' message", err.Error())
	}
}

func TestResolveAskPrompt_EmptyArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("prompt", "", "")
	_, err := resolveAskPrompt(cmd, []string{"  "})
	if err == nil {
		t.Fatal("expected error when empty args provided")
	}
}

func TestAskRejectsAgentContext(t *testing.T) {
	t.Setenv("ADAF_AGENT", "1")
	cmd := &cobra.Command{}
	cmd.Flags().String("agent", "claude", "")
	cmd.Flags().String("profile", "", "")
	cmd.Flags().String("prompt", "test", "")
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("command", "", "")
	cmd.Flags().String("reasoning-level", "", "")
	cmd.Flags().String("plan", "", "")
	cmd.Flags().Int("count", 1, "")
	cmd.Flags().Bool("chain", false, "")
	cmd.Flags().BoolP("session", "s", false, "")
	err := runAsk(cmd, []string{"test prompt"})
	if err == nil {
		t.Fatal("runAsk() error = nil, want agent-context rejection")
	}
	if !strings.Contains(err.Error(), "not available inside an agent context") {
		t.Fatalf("runAsk() error = %q, want context rejection message", err.Error())
	}
}

func TestAskFlagsExist(t *testing.T) {
	flags := []string{
		"agent", "profile", "prompt", "model", "command",
		"reasoning-level", "plan", "count", "chain", "session",
	}
	for _, name := range flags {
		if askCmd.Flags().Lookup(name) == nil {
			t.Errorf("ask --%s flag missing", name)
		}
	}
}

func TestAskCmdRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "ask [prompt]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ask command not registered on rootCmd")
	}
}
