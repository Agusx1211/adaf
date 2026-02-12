package agent

import (
	"reflect"
	"testing"

	"github.com/agusx1211/adaf/internal/config"
)

func TestBuildLaunchSpecVibeUsesEnvModel(t *testing.T) {
	spec := BuildLaunchSpec(&config.Profile{
		Agent: "vibe",
		Model: "qwen-coder-next",
	}, nil, "")

	if got := spec.Command; got != "" {
		t.Fatalf("Command = %q, want empty for built-in vibe", got)
	}
	if len(spec.Args) != 0 {
		t.Fatalf("Args = %v, want no args for vibe model selection", spec.Args)
	}
	if got := spec.Env["VIBE_ACTIVE_MODEL"]; got != "qwen-coder-next" {
		t.Fatalf("VIBE_ACTIVE_MODEL = %q, want %q", got, "qwen-coder-next")
	}
}

func TestBuildLaunchSpecCodexArgs(t *testing.T) {
	spec := BuildLaunchSpec(&config.Profile{
		Agent:          "codex",
		Model:          "gpt-5.3-codex",
		ReasoningLevel: "xhigh",
	}, nil, "")

	wantArgs := []string{
		"--model", "gpt-5.3-codex",
		"-c", `model_reasoning_effort="xhigh"`,
		"--dangerously-bypass-approvals-and-sandbox",
	}
	if !reflect.DeepEqual(spec.Args, wantArgs) {
		t.Fatalf("Args = %#v, want %#v", spec.Args, wantArgs)
	}
	if spec.Env != nil {
		t.Fatalf("Env = %#v, want nil", spec.Env)
	}
}

func TestBuildLaunchSpecCommandResolution(t *testing.T) {
	tests := []struct {
		name            string
		profile         *config.Profile
		agentsCfg       *AgentsConfig
		commandOverride string
		wantCommand     string
	}{
		{
			name: "override takes precedence",
			profile: &config.Profile{
				Agent: "vibe",
			},
			commandOverride: "/custom/vibe",
			wantCommand:     "/custom/vibe",
		},
		{
			name: "agents config path used when present",
			profile: &config.Profile{
				Agent: "vibe",
			},
			agentsCfg: &AgentsConfig{
				Agents: map[string]AgentRecord{
					"vibe": {Path: "/opt/vibe/bin/vibe"},
				},
			},
			wantCommand: "/opt/vibe/bin/vibe",
		},
		{
			name: "built-in agent keeps empty command fallback",
			profile: &config.Profile{
				Agent: "claude",
			},
			wantCommand: "",
		},
		{
			name: "custom agent falls back to agent name",
			profile: &config.Profile{
				Agent: "my-agent",
			},
			wantCommand: "my-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := BuildLaunchSpec(tt.profile, tt.agentsCfg, tt.commandOverride)
			if spec.Command != tt.wantCommand {
				t.Fatalf("Command = %q, want %q", spec.Command, tt.wantCommand)
			}
		})
	}
}
