package looprun

import (
	"strings"

	"github.com/agusx1211/adaf/internal/config"
	promptpkg "github.com/agusx1211/adaf/internal/prompt"
	"github.com/agusx1211/adaf/internal/store"
)

// StepPromptInput contains everything needed to generate the exact loop step
// prompt used by the runtime for non-resume turns.
type StepPromptInput struct {
	Store     *store.Store
	Project   *store.ProjectConfig
	GlobalCfg *config.GlobalConfig

	PlanID        string
	InitialPrompt string

	LoopName   string
	RunID      int
	Cycle      int
	StepIndex  int
	TotalSteps int

	Step       config.LoopStep
	Profile    *config.Profile
	Delegation *config.DelegationConfig

	Messages []store.LoopMessage
	Handoffs []store.HandoffInfo

	CurrentTurnID   int
	ResumeSessionID string
}

// BuildStepPrompt builds the loop prompt with the same inputs and behavior
// used by looprun.Run for non-resume turns.
func BuildStepPrompt(input StepPromptInput) (string, error) {
	// When resuming a standalone chat, the runtime sends only the user message.
	if strings.TrimSpace(input.ResumeSessionID) != "" && input.Step.StandaloneChat {
		return input.Step.Instructions, nil
	}

	totalSteps := input.TotalSteps
	if totalSteps <= 0 {
		totalSteps = 1
	}

	loopName := strings.TrimSpace(input.LoopName)
	if loopName == "" {
		loopName = "loop"
	}

	position := config.EffectiveStepPosition(input.Step)
	workerRole := config.EffectiveWorkerRoleForPosition(position, input.Step.Role, input.GlobalCfg)
	loopCtx := &promptpkg.LoopPromptContext{
		LoopName:      loopName,
		Cycle:         input.Cycle,
		StepIndex:     input.StepIndex,
		TotalSteps:    totalSteps,
		Instructions:  input.Step.Instructions,
		InitialPrompt: input.InitialPrompt,
		CanStop:       input.Step.CanStop,
		CanMessage:    input.Step.CanMessage,
		CanPushover:   input.Step.CanPushover,
		Messages:      input.Messages,
		RunID:         input.RunID,
	}

	opts := promptpkg.BuildOpts{
		Store:          input.Store,
		Project:        input.Project,
		PlanID:         input.PlanID,
		Profile:        input.Profile,
		Role:           workerRole,
		Position:       position,
		GlobalCfg:      input.GlobalCfg,
		CurrentTurnID:  input.CurrentTurnID,
		LoopContext:    loopCtx,
		Delegation:     input.Delegation,
		Handoffs:       input.Handoffs,
		StandaloneChat: input.Step.StandaloneChat,
		Skills:         input.Step.Skills,
	}
	return promptpkg.Build(opts)
}
