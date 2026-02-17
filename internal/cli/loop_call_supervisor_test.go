package cli

import (
	"fmt"
	"strings"
	"testing"

	loopctrl "github.com/agusx1211/adaf/internal/loop"
	"github.com/agusx1211/adaf/internal/store"
)

func TestLoopCallSupervisor_SignalsFastForwardAndInterrupt(t *testing.T) {
	s, projectDir := newLoopCommandTestStore(t)
	run := createLoopCommandTestRun(t, s, []store.LoopRunStep{
		{Profile: "mgr", Position: "manager"},
		{Profile: "dev", Position: "lead"},
		{Profile: "sup", Position: "supervisor"},
	})

	t.Setenv("ADAF_PROJECT_DIR", projectDir)
	t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", run.ID))
	t.Setenv("ADAF_LOOP_STEP_INDEX", "0")
	t.Setenv("ADAF_POSITION", "manager")
	t.Setenv("ADAF_TURN_ID", "4242")

	if err := loopCallSupervisor(nil, []string{"Need supervisor decision"}); err != nil {
		t.Fatalf("loopCallSupervisor() error = %v", err)
	}

	msgs, err := s.ListLoopMessages(run.ID)
	if err != nil {
		t.Fatalf("ListLoopMessages() error = %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "Need supervisor decision" {
		t.Fatalf("message content = %q, want %q", msgs[0].Content, "Need supervisor decision")
	}
	if msgs[0].StepIndex != 0 {
		t.Fatalf("message step index = %d, want 0", msgs[0].StepIndex)
	}

	sig, err := s.GetLoopCallSupervisorSignal(run.ID)
	if err != nil {
		t.Fatalf("GetLoopCallSupervisorSignal() error = %v", err)
	}
	if sig == nil {
		t.Fatal("signal is nil, want pending call-supervisor signal")
	}
	if sig.FromStepIndex != 0 {
		t.Fatalf("signal from step = %d, want 0", sig.FromStepIndex)
	}
	if sig.TargetStepIndex != 2 {
		t.Fatalf("signal target step = %d, want 2", sig.TargetStepIndex)
	}

	if got := s.CheckInterrupt(4242); got != loopctrl.InterruptMessageCallSupervisor {
		t.Fatalf("interrupt message = %q, want %q", got, loopctrl.InterruptMessageCallSupervisor)
	}
}

func TestLoopCallSupervisor_UnavailableWithoutSupervisor(t *testing.T) {
	s, projectDir := newLoopCommandTestStore(t)
	run := createLoopCommandTestRun(t, s, []store.LoopRunStep{
		{Profile: "mgr", Position: "manager"},
		{Profile: "dev", Position: "lead"},
	})

	t.Setenv("ADAF_PROJECT_DIR", projectDir)
	t.Setenv("ADAF_LOOP_RUN_ID", fmt.Sprintf("%d", run.ID))
	t.Setenv("ADAF_LOOP_STEP_INDEX", "0")
	t.Setenv("ADAF_POSITION", "manager")

	err := loopCallSupervisor(nil, []string{"Need supervisor decision"})
	if err == nil {
		t.Fatal("loopCallSupervisor() error = nil, want unavailable error")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error = %q, want unavailable", err.Error())
	}

	sig, err := s.GetLoopCallSupervisorSignal(run.ID)
	if err != nil {
		t.Fatalf("GetLoopCallSupervisorSignal() error = %v", err)
	}
	if sig != nil {
		t.Fatalf("signal = %+v, want nil", sig)
	}
}

func TestNextSupervisorStepIndexInLoopRun_WrapsToNextCycle(t *testing.T) {
	run := &store.LoopRun{
		Steps: []store.LoopRunStep{
			{Profile: "sup", Position: "supervisor"},
			{Profile: "lead", Position: "lead"},
			{Profile: "mgr", Position: "manager"},
		},
	}

	target, ok := nextSupervisorStepIndexInLoopRun(run, 2)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if target != 0 {
		t.Fatalf("target = %d, want 0", target)
	}
}

func newLoopCommandTestStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	projectDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "loop-cli-test", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	return s, projectDir
}

func createLoopCommandTestRun(t *testing.T, s *store.Store, steps []store.LoopRunStep) *store.LoopRun {
	t.Helper()
	run := &store.LoopRun{
		LoopName:        "loop-cli-test",
		Steps:           steps,
		StepLastSeenMsg: map[int]int{},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun() error = %v", err)
	}
	return run
}
