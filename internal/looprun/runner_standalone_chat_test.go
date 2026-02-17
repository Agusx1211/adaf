package looprun

import (
	"context"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/events"
)

func TestBuildAgentConfig_ResumeSessionIDPassedThrough(t *testing.T) {
	prof := &config.Profile{Name: "chat-profile", Agent: "generic"}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/echo"},
		},
	}

	cfg := RunConfig{
		WorkDir:         "/tmp/workdir",
		AgentsCfg:       agentsCfg,
		SessionID:       5,
		ResumeSessionID: "claude-session-xyz",
	}

	ac := buildAgentConfig(cfg, prof, config.LoopStep{Position: config.PositionLead}, 1, 0, "run-hex", "step-hex", nil)

	if ac.ResumeSessionID != "claude-session-xyz" {
		t.Fatalf("ResumeSessionID = %q, want %q", ac.ResumeSessionID, "claude-session-xyz")
	}
}

func TestBuildAgentConfig_EmptyResumeSessionID(t *testing.T) {
	prof := &config.Profile{Name: "p1", Agent: "generic"}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/echo"},
		},
	}

	cfg := RunConfig{
		WorkDir:   "/tmp",
		AgentsCfg: agentsCfg,
	}

	ac := buildAgentConfig(cfg, prof, config.LoopStep{Position: config.PositionLead}, 1, 0, "", "", nil)

	if ac.ResumeSessionID != "" {
		t.Fatalf("ResumeSessionID = %q, want empty", ac.ResumeSessionID)
	}
}

func TestRun_StandaloneChatResumeUsesInstructionsAsPrompt(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}

	userMessage := "What is 2+5?"
	loopDef := &config.LoopDef{
		Name: "standalone-chat",
		Steps: []config.LoopStep{
			{
				Profile:        "p1",
				Turns:          1,
				Instructions:   userMessage,
				StandaloneChat: true,
			},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/cat"},
		},
	}

	eventCh := make(chan any, 256)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = Run(ctx, RunConfig{
		Store:           s,
		GlobalCfg:       globalCfg,
		LoopDef:         loopDef,
		Project:         proj,
		AgentsCfg:       agentsCfg,
		WorkDir:         proj.RepoPath,
		MaxCycles:       1,
		ResumeSessionID: "prev-agent-session-id",
	}, eventCh)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Drain events and find the prompt event.
	// The prompt should be the raw user message, NOT a full Build() output.
	close(eventCh)
	var promptMsg *events.AgentPromptMsg
	for ev := range eventCh {
		if pm, ok := ev.(events.AgentPromptMsg); ok {
			promptMsg = &pm
		}
	}

	if promptMsg == nil {
		t.Fatal("no agent_prompt event found in event channel")
	}

	// The prompt should be exactly the user message (or very close to it),
	// not a full Build() output with role headers, tools section, etc.
	if promptMsg.Prompt != userMessage {
		t.Fatalf("prompt = %q, want %q (raw user message, not full Build output)", promptMsg.Prompt, userMessage)
	}
}

func TestRun_StandaloneChatWithoutResumeUsesFullPrompt(t *testing.T) {
	s := newLooprunTestStore(t)
	proj, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}

	globalCfg := &config.GlobalConfig{
		Profiles: []config.Profile{
			{Name: "p1", Agent: "generic"},
		},
	}

	userMessage := "hello"
	loopDef := &config.LoopDef{
		Name: "standalone-chat",
		Steps: []config.LoopStep{
			{
				Profile:        "p1",
				Turns:          1,
				Instructions:   userMessage,
				StandaloneChat: true,
			},
		},
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/cat"},
		},
	}

	eventCh := make(chan any, 256)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// No ResumeSessionID — should use full Build() path.
	err = Run(ctx, RunConfig{
		Store:     s,
		GlobalCfg: globalCfg,
		LoopDef:   loopDef,
		Project:   proj,
		AgentsCfg: agentsCfg,
		WorkDir:   proj.RepoPath,
		MaxCycles: 1,
	}, eventCh)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	close(eventCh)
	var promptMsg *events.AgentPromptMsg
	for ev := range eventCh {
		if pm, ok := ev.(events.AgentPromptMsg); ok {
			promptMsg = &pm
		}
	}

	if promptMsg == nil {
		t.Fatal("no agent_prompt event found in event channel")
	}

	// Without resume, the prompt should include the standalone chat context
	// (role identity, tools section, etc.) — NOT just the raw user message.
	if promptMsg.Prompt == userMessage {
		t.Fatal("prompt should include full standalone chat context, not just the raw user message")
	}

	// Should contain the standalone chat marker text.
	if len(promptMsg.Prompt) < len(userMessage)+10 {
		t.Fatalf("prompt too short to contain standalone chat context: len=%d", len(promptMsg.Prompt))
	}
}
