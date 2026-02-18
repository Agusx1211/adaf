package store

import "testing"

func TestLoopCallSupervisorSignalLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(ProjectConfig{Name: "loop-signal-test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	run := &LoopRun{
		LoopName:        "loop-signal-test",
		Steps:           []LoopRunStep{{Profile: "manager"}, {Profile: "supervisor", Position: "supervisor"}},
		StepLastSeenMsg: map[int]int{},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun() error = %v", err)
	}

	if err := s.SignalLoopCallSupervisor(run.ID, 0, 1, "need review"); err != nil {
		t.Fatalf("SignalLoopCallSupervisor() error = %v", err)
	}

	got, err := s.GetLoopCallSupervisorSignal(run.ID)
	if err != nil {
		t.Fatalf("GetLoopCallSupervisorSignal() error = %v", err)
	}
	if got == nil {
		t.Fatal("signal = nil, want non-nil")
	}
	if got.FromStepIndex != 0 || got.TargetStepIndex != 1 {
		t.Fatalf("signal indexes = (%d,%d), want (0,1)", got.FromStepIndex, got.TargetStepIndex)
	}
	if got.Content != "need review" {
		t.Fatalf("signal content = %q, want %q", got.Content, "need review")
	}

	consumed, err := s.ConsumeLoopCallSupervisorSignal(run.ID)
	if err != nil {
		t.Fatalf("ConsumeLoopCallSupervisorSignal() error = %v", err)
	}
	if consumed == nil {
		t.Fatal("consumed signal = nil, want non-nil")
	}
	if consumed.TargetStepIndex != 1 {
		t.Fatalf("consumed target = %d, want 1", consumed.TargetStepIndex)
	}

	again, err := s.GetLoopCallSupervisorSignal(run.ID)
	if err != nil {
		t.Fatalf("GetLoopCallSupervisorSignal(after consume) error = %v", err)
	}
	if again != nil {
		t.Fatalf("signal after consume = %+v, want nil", again)
	}
}

func TestLoopWindDownSignalLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("store.New() error = %v", err)
	}
	if err := s.Init(ProjectConfig{Name: "loop-wind-down-test", RepoPath: dir}); err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}

	run := &LoopRun{
		LoopName:        "loop-wind-down-test",
		Steps:           []LoopRunStep{{Profile: "lead"}},
		StepLastSeenMsg: map[int]int{},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun() error = %v", err)
	}

	if s.IsLoopWindDown(run.ID) {
		t.Fatalf("IsLoopWindDown(%d) = true before signal, want false", run.ID)
	}
	if err := s.SignalLoopWindDown(run.ID); err != nil {
		t.Fatalf("SignalLoopWindDown() error = %v", err)
	}
	if !s.IsLoopWindDown(run.ID) {
		t.Fatalf("IsLoopWindDown(%d) = false after signal, want true", run.ID)
	}
}
