package looprun

import (
	"strings"
	"testing"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestGatherUnseenMessages_NoMessages(t *testing.T) {
	s := newLooprunTestStore(t)
	run := &store.LoopRun{
		StepLastSeenMsg: make(map[int]int),
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun: %v", err)
	}

	msgs := gatherUnseenMessages(s, run, 0)
	if len(msgs) != 0 {
		t.Fatalf("msgs = %d, want 0", len(msgs))
	}
}

func TestGatherUnseenMessages_FiltersOwnStepMessages(t *testing.T) {
	s := newLooprunTestStore(t)
	run := &store.LoopRun{
		StepLastSeenMsg: make(map[int]int),
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun: %v", err)
	}

	// Messages from step 0 (the step requesting) should be excluded.
	_ = s.CreateLoopMessage(&store.LoopMessage{RunID: run.ID, StepIndex: 0, Content: "own message"})
	_ = s.CreateLoopMessage(&store.LoopMessage{RunID: run.ID, StepIndex: 1, Content: "from step 1"})

	msgs := gatherUnseenMessages(s, run, 0)
	if len(msgs) != 1 {
		t.Fatalf("msgs = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "from step 1" {
		t.Fatalf("msg content = %q, want %q", msgs[0].Content, "from step 1")
	}
}

func TestGatherUnseenMessages_RespectsWatermark(t *testing.T) {
	s := newLooprunTestStore(t)
	run := &store.LoopRun{
		StepLastSeenMsg: map[int]int{0: 0},
	}
	if err := s.CreateLoopRun(run); err != nil {
		t.Fatalf("CreateLoopRun: %v", err)
	}

	_ = s.CreateLoopMessage(&store.LoopMessage{RunID: run.ID, StepIndex: 1, Content: "msg1"})
	_ = s.CreateLoopMessage(&store.LoopMessage{RunID: run.ID, StepIndex: 1, Content: "msg2"})

	// First call: should see both messages.
	msgs := gatherUnseenMessages(s, run, 0)
	if len(msgs) != 2 {
		t.Fatalf("first: msgs = %d, want 2", len(msgs))
	}

	// Set watermark to last seen message.
	run.StepLastSeenMsg[0] = msgs[len(msgs)-1].ID

	// Second call: no new messages.
	msgs = gatherUnseenMessages(s, run, 0)
	if len(msgs) != 0 {
		t.Fatalf("after watermark: msgs = %d, want 0", len(msgs))
	}

	// Third message arrives.
	_ = s.CreateLoopMessage(&store.LoopMessage{RunID: run.ID, StepIndex: 2, Content: "new msg"})
	msgs = gatherUnseenMessages(s, run, 0)
	if len(msgs) != 1 {
		t.Fatalf("after new msg: msgs = %d, want 1", len(msgs))
	}
	if msgs[0].Content != "new msg" {
		t.Fatalf("msg content = %q, want %q", msgs[0].Content, "new msg")
	}
}

func TestCollectHandoffs_EmptyTurnIDs(t *testing.T) {
	s := newLooprunTestStore(t)
	handoffs := collectHandoffs(s, nil)
	if len(handoffs) != 0 {
		t.Fatalf("handoffs = %d, want 0", len(handoffs))
	}
}

func TestCollectHandoffs_OnlyRunningHandoffs(t *testing.T) {
	s := newLooprunTestStore(t)

	// Create handoff spawn (running).
	handoffRec := &store.SpawnRecord{
		ParentTurnID:  10,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "handoff task",
		Status:        "running",
		Handoff:       true,
		Speed:         "fast",
		Branch:        "adaf/handoff-1",
	}
	if err := s.CreateSpawn(handoffRec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	// Create non-handoff spawn (running).
	normalRec := &store.SpawnRecord{
		ParentTurnID:  10,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "normal task",
		Status:        "running",
		Handoff:       false,
	}
	if err := s.CreateSpawn(normalRec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	// Create completed handoff spawn.
	completedRec := &store.SpawnRecord{
		ParentTurnID:  10,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "done task",
		Status:        "completed",
		Handoff:       true,
	}
	if err := s.CreateSpawn(completedRec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	handoffs := collectHandoffs(s, []int{10})
	if len(handoffs) != 1 {
		t.Fatalf("handoffs = %d, want 1", len(handoffs))
	}
	if handoffs[0].SpawnID != handoffRec.ID {
		t.Fatalf("handoff spawn ID = %d, want %d", handoffs[0].SpawnID, handoffRec.ID)
	}
	if handoffs[0].Profile != "worker" {
		t.Fatalf("handoff profile = %q, want %q", handoffs[0].Profile, "worker")
	}
	if handoffs[0].Speed != "fast" {
		t.Fatalf("handoff speed = %q, want %q", handoffs[0].Speed, "fast")
	}
}

func TestCollectHandoffs_DeduplicatesAcrossTurnIDs(t *testing.T) {
	s := newLooprunTestStore(t)

	rec := &store.SpawnRecord{
		ParentTurnID:  20,
		ParentProfile: "parent",
		ChildProfile:  "worker",
		Task:          "handoff",
		Status:        "running",
		Handoff:       true,
	}
	if err := s.CreateSpawn(rec); err != nil {
		t.Fatalf("CreateSpawn: %v", err)
	}

	// Pass the same turn ID twice; should not duplicate.
	handoffs := collectHandoffs(s, []int{20, 20})
	if len(handoffs) != 1 {
		t.Fatalf("handoffs = %d, want 1 (deduped)", len(handoffs))
	}
}

func TestTruncatePromptForEvent_ShortPrompt(t *testing.T) {
	prompt := "short prompt"
	result, truncated, origLen := truncatePromptForEvent(prompt)
	if truncated {
		t.Fatal("short prompt should not be truncated")
	}
	if result != prompt {
		t.Fatalf("result = %q, want %q", result, prompt)
	}
	if origLen != len(prompt) {
		t.Fatalf("origLen = %d, want %d", origLen, len(prompt))
	}
}

func TestTruncatePromptForEvent_LongPrompt(t *testing.T) {
	// Build a string larger than promptEventLimitBytes (256KB).
	prompt := string(make([]byte, 300*1024))
	for i := range prompt {
		_ = i // fill with zeros is fine for length test
	}

	result, truncated, origLen := truncatePromptForEvent(prompt)
	if !truncated {
		t.Fatal("long prompt should be truncated")
	}
	if origLen != 300*1024 {
		t.Fatalf("origLen = %d, want %d", origLen, 300*1024)
	}
	if len(result) < promptEventLimitBytes {
		t.Fatalf("result length = %d, want >= %d", len(result), promptEventLimitBytes)
	}
}

func TestBuildStepPrompt_IncludesLoopAndProjectContext(t *testing.T) {
	s := newLooprunTestStore(t)
	project, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject() error = %v", err)
	}

	globalCfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(globalCfg)
	config.EnsureDefaultSkillCatalog(globalCfg)

	prof := &config.Profile{Name: "p", Agent: "generic"}
	step := config.LoopStep{
		Profile:      "p",
		Position:     config.PositionLead,
		Instructions: "Ship the selected feature.",
		CanMessage:   true,
	}

	prompt, err := BuildStepPrompt(StepPromptInput{
		Store:            s,
		Project:          project,
		GlobalCfg:        globalCfg,
		ResourcePriority: config.ResourcePriorityCost,
		LoopName:         "preview-loop",
		Cycle:            0,
		StepIndex:        1,
		TotalSteps:       3,
		Step:             step,
		Profile:          prof,
		CurrentTurnID:    0,
	})
	if err != nil {
		t.Fatalf("BuildStepPrompt() error = %v", err)
	}
	if !strings.Contains(prompt, "preview-loop") {
		t.Fatalf("prompt missing loop name:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Project: test") {
		t.Fatalf("prompt missing project name:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Ship the selected feature.") {
		t.Fatalf("prompt missing step instructions:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Current priority: **cost**") {
		t.Fatalf("prompt missing resource-priority context:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Prefer `free`/`cheap` spawn profiles") {
		t.Fatalf("prompt missing cost-priority routing guidance:\n%s", prompt)
	}
}

func TestBuildStepPrompt_StandaloneResumeReturnsInstructionsOnly(t *testing.T) {
	prompt, err := BuildStepPrompt(StepPromptInput{
		ResumeSessionID: "sess-123",
		Step: config.LoopStep{
			StandaloneChat: true,
			Instructions:   "user follow-up",
		},
	})
	if err != nil {
		t.Fatalf("BuildStepPrompt() error = %v", err)
	}
	if prompt != "user follow-up" {
		t.Fatalf("prompt = %q, want %q", prompt, "user follow-up")
	}
}

func TestBuildStepPrompt_ExplicitEmptySkillsDisableDefaults(t *testing.T) {
	s := newLooprunTestStore(t)
	project, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject() error = %v", err)
	}

	globalCfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(globalCfg)
	config.EnsureDefaultSkillCatalog(globalCfg)

	prof := &config.Profile{Name: "p", Agent: "generic"}
	step := config.LoopStep{
		Profile:        "p",
		Position:       config.PositionLead,
		SkillsExplicit: true,
	}

	prompt, err := BuildStepPrompt(StepPromptInput{
		Store:      s,
		Project:    project,
		GlobalCfg:  globalCfg,
		LoopName:   "skills-none",
		Step:       step,
		Profile:    prof,
		TotalSteps: 1,
	})
	if err != nil {
		t.Fatalf("BuildStepPrompt() error = %v", err)
	}
	if strings.Contains(prompt, "# Skills") {
		t.Fatalf("prompt should not include default skills when skills_explicit=true and no skills selected:\n%s", prompt)
	}
	if !strings.Contains(prompt, "There is no human in the loop.") {
		t.Fatalf("loop prompt should always state there is no human in the loop:\n%s", prompt)
	}
}

func TestBuildStepPrompt_ManagerCallSupervisorAvailabilityFollowsLoopSupervisorPresence(t *testing.T) {
	s := newLooprunTestStore(t)
	project, err := s.LoadProject()
	if err != nil {
		t.Fatalf("LoadProject() error = %v", err)
	}

	globalCfg := &config.GlobalConfig{}
	config.EnsureDefaultRoleCatalog(globalCfg)
	config.EnsureDefaultSkillCatalog(globalCfg)

	prof := &config.Profile{Name: "manager", Agent: "generic"}
	step := config.LoopStep{
		Profile:  "manager",
		Position: config.PositionManager,
	}

	withSupervisorPrompt, err := BuildStepPrompt(StepPromptInput{
		Store:      s,
		Project:    project,
		GlobalCfg:  globalCfg,
		LoopName:   "with-supervisor",
		Step:       step,
		LoopSteps:  []config.LoopStep{{Profile: "manager", Position: config.PositionManager}, {Profile: "supervisor", Position: config.PositionSupervisor}},
		Profile:    prof,
		TotalSteps: 2,
	})
	if err != nil {
		t.Fatalf("BuildStepPrompt(with supervisor) error = %v", err)
	}
	if !strings.Contains(withSupervisorPrompt, "adaf loop call-supervisor") {
		t.Fatalf("prompt should include call-supervisor when loop has supervisor:\n%s", withSupervisorPrompt)
	}

	withoutSupervisorPrompt, err := BuildStepPrompt(StepPromptInput{
		Store:      s,
		Project:    project,
		GlobalCfg:  globalCfg,
		LoopName:   "without-supervisor",
		Step:       step,
		LoopSteps:  []config.LoopStep{{Profile: "manager", Position: config.PositionManager}, {Profile: "lead", Position: config.PositionLead}},
		Profile:    prof,
		TotalSteps: 2,
	})
	if err != nil {
		t.Fatalf("BuildStepPrompt(without supervisor) error = %v", err)
	}
	if strings.Contains(withoutSupervisorPrompt, "adaf loop call-supervisor") {
		t.Fatalf("prompt should omit call-supervisor when loop has no supervisor:\n%s", withoutSupervisorPrompt)
	}
}

func TestBuildAgentConfig_SetsEnvironmentVariables(t *testing.T) {
	prof := &config.Profile{
		Name:  "test-profile",
		Agent: "generic",
	}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {
				Name: "generic",
				Path: "/bin/echo",
			},
		},
	}

	cfg := RunConfig{
		WorkDir:   "/tmp/workdir",
		AgentsCfg: agentsCfg,
		SessionID: 42,
	}

	ac := buildAgentConfig(cfg, prof, config.LoopStep{Position: config.PositionLead}, 7, 2, "run-hex", "step-hex", nil)

	if ac.WorkDir != "/tmp/workdir" {
		t.Fatalf("WorkDir = %q, want %q", ac.WorkDir, "/tmp/workdir")
	}
	if ac.Env["ADAF_LOOP_RUN_ID"] != "7" {
		t.Fatalf("ADAF_LOOP_RUN_ID = %q, want %q", ac.Env["ADAF_LOOP_RUN_ID"], "7")
	}
	if ac.Env["ADAF_LOOP_STEP_INDEX"] != "2" {
		t.Fatalf("ADAF_LOOP_STEP_INDEX = %q, want %q", ac.Env["ADAF_LOOP_STEP_INDEX"], "2")
	}
	if ac.Env["ADAF_SESSION_ID"] != "42" {
		t.Fatalf("ADAF_SESSION_ID = %q, want %q", ac.Env["ADAF_SESSION_ID"], "42")
	}
	if ac.Env["ADAF_LOOP_RUN_HEX_ID"] != "run-hex" {
		t.Fatalf("ADAF_LOOP_RUN_HEX_ID = %q, want %q", ac.Env["ADAF_LOOP_RUN_HEX_ID"], "run-hex")
	}
	if ac.Env["ADAF_LOOP_STEP_HEX_ID"] != "step-hex" {
		t.Fatalf("ADAF_LOOP_STEP_HEX_ID = %q, want %q", ac.Env["ADAF_LOOP_STEP_HEX_ID"], "step-hex")
	}
	if ac.Env["ADAF_POSITION"] != config.PositionLead {
		t.Fatalf("ADAF_POSITION = %q, want %q", ac.Env["ADAF_POSITION"], config.PositionLead)
	}
	if ac.Env["ADAF_RESOURCE_PRIORITY"] != config.ResourcePriorityNormal {
		t.Fatalf("ADAF_RESOURCE_PRIORITY = %q, want %q", ac.Env["ADAF_RESOURCE_PRIORITY"], config.ResourcePriorityNormal)
	}
	if ac.Name != "generic" {
		t.Fatalf("Name = %q, want %q", ac.Name, "generic")
	}
}

func TestBuildAgentConfig_OmitsSessionIDWhenZero(t *testing.T) {
	prof := &config.Profile{Name: "test", Agent: "generic"}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/echo"},
		},
	}
	cfg := RunConfig{
		WorkDir:   "/tmp",
		AgentsCfg: agentsCfg,
		SessionID: 0,
	}

	ac := buildAgentConfig(cfg, prof, config.LoopStep{Position: config.PositionLead}, 1, 0, "", "", nil)
	if _, ok := ac.Env["ADAF_SESSION_ID"]; ok {
		t.Fatalf("ADAF_SESSION_ID should not be set when SessionID is 0")
	}
}

func TestBuildAgentConfig_SetsDelegationJSON(t *testing.T) {
	prof := &config.Profile{Name: "test", Agent: "generic"}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/echo"},
		},
	}
	cfg := RunConfig{
		WorkDir:   "/tmp",
		AgentsCfg: agentsCfg,
	}

	deleg := &config.DelegationConfig{
		Profiles: []config.DelegationProfile{
			{Name: "worker", Role: "coder"},
		},
		MaxParallel: 2,
	}

	ac := buildAgentConfig(cfg, prof, config.LoopStep{Position: config.PositionLead}, 1, 0, "", "", deleg)
	val, ok := ac.Env["ADAF_DELEGATION_JSON"]
	if !ok {
		t.Fatal("ADAF_DELEGATION_JSON not set when delegation is non-nil")
	}
	if val == "" {
		t.Fatal("ADAF_DELEGATION_JSON is empty")
	}

	// Verify it's valid JSON containing expected data.
	if !strings.Contains(val, "worker") {
		t.Fatalf("ADAF_DELEGATION_JSON = %q, want it to contain %q", val, "worker")
	}
}

func TestBuildAgentConfig_NoDelegationJSON(t *testing.T) {
	prof := &config.Profile{Name: "test", Agent: "generic"}
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
	if _, ok := ac.Env["ADAF_DELEGATION_JSON"]; ok {
		t.Fatal("ADAF_DELEGATION_JSON should not be set when delegation is nil")
	}
}

func TestBuildAgentConfig_UsesLoopResourcePriority(t *testing.T) {
	prof := &config.Profile{Name: "test", Agent: "generic"}
	agentsCfg := &agent.AgentsConfig{
		Agents: map[string]agent.AgentRecord{
			"generic": {Name: "generic", Path: "/bin/echo"},
		},
	}
	cfg := RunConfig{
		WorkDir:   "/tmp",
		AgentsCfg: agentsCfg,
		LoopDef: &config.LoopDef{
			Name:             "prio-loop",
			ResourcePriority: config.ResourcePriorityCost,
		},
	}

	ac := buildAgentConfig(cfg, prof, config.LoopStep{Position: config.PositionLead}, 1, 0, "", "", nil)
	if ac.Env["ADAF_RESOURCE_PRIORITY"] != config.ResourcePriorityCost {
		t.Fatalf("ADAF_RESOURCE_PRIORITY = %q, want %q", ac.Env["ADAF_RESOURCE_PRIORITY"], config.ResourcePriorityCost)
	}
}

func TestNextStepResumeSessionID_StandaloneUsesBaseResume(t *testing.T) {
	prof := &config.Profile{Name: "p1", Agent: "codex"}
	step := config.LoopStep{StandaloneChat: true}
	prev := roleResumeState{Position: config.PositionManager, Role: "", Agent: "codex", SessionID: "prev-sess"}

	got := nextStepResumeSessionID("standalone-sess", step, prof, prev)
	if got != "standalone-sess" {
		t.Fatalf("nextStepResumeSessionID() = %q, want %q", got, "standalone-sess")
	}
}

func TestNextStepResumeSessionID_ResumesOnlyWhenRoleAndAgentMatch(t *testing.T) {
	prof := &config.Profile{Name: "p1", Agent: "codex"}
	step := config.LoopStep{Position: config.PositionManager}
	prev := roleResumeState{Position: config.PositionManager, Role: "", Agent: "codex", SessionID: "prev-sess"}

	got := nextStepResumeSessionID("", step, prof, prev)
	if got != "prev-sess" {
		t.Fatalf("nextStepResumeSessionID() = %q, want %q", got, "prev-sess")
	}
}

func TestNextStepResumeSessionID_DoesNotResumeOnRoleOrAgentMismatch(t *testing.T) {
	prof := &config.Profile{Name: "p1", Agent: "codex"}

	byRole := nextStepResumeSessionID("", config.LoopStep{Position: config.PositionSupervisor}, prof, roleResumeState{
		Position:  config.PositionManager,
		Role:      "",
		Agent:     "codex",
		SessionID: "prev-sess",
	})
	if byRole != "" {
		t.Fatalf("role mismatch resume id = %q, want empty", byRole)
	}

	byAgent := nextStepResumeSessionID("", config.LoopStep{Position: config.PositionManager}, prof, roleResumeState{
		Position:  config.PositionManager,
		Role:      "",
		Agent:     "claude",
		SessionID: "prev-sess",
	})
	if byAgent != "" {
		t.Fatalf("agent mismatch resume id = %q, want empty", byAgent)
	}
}

func TestNextRoleResumeState_RequiresRoleAgentAndSessionID(t *testing.T) {
	prof := &config.Profile{Name: "p1", Agent: "codex"}
	step := config.LoopStep{Position: config.PositionManager}

	state := nextRoleResumeState(step, prof, "new-sess")
	if state.Position != config.PositionManager || state.Agent != "codex" || state.SessionID != "new-sess" {
		t.Fatalf("nextRoleResumeState() = %+v, want role+agent+session", state)
	}

	empty := nextRoleResumeState(config.LoopStep{}, nil, "new-sess")
	if empty != (roleResumeState{}) {
		t.Fatalf("nextRoleResumeState() without agent = %+v, want empty", empty)
	}
}

func TestSpawnSnapshotFingerprint_Empty(t *testing.T) {
	fp := spawnSnapshotFingerprint(nil)
	if fp != "" {
		t.Fatalf("fingerprint = %q, want empty", fp)
	}
}

func TestWaitForAnySessionSpawns_NoSpawns(t *testing.T) {
	s := newLooprunTestStore(t)
	results, morePending := waitForAnySessionSpawns(t.Context(), s, 999, nil)
	if len(results) != 0 {
		t.Fatalf("results = %d, want 0", len(results))
	}
	if morePending {
		t.Fatal("morePending = true, want false")
	}
}

func TestWaitForAnySessionSpawns_AllAlreadySeen(t *testing.T) {
	s := newLooprunTestStore(t)
	parentTurnID := 50

	rec := createLooprunSpawn(t, s, parentTurnID, "completed")
	alreadySeen := map[int]struct{}{rec.ID: {}}

	results, morePending := waitForAnySessionSpawns(t.Context(), s, parentTurnID, alreadySeen)
	if len(results) != 0 {
		t.Fatalf("results = %d, want 0", len(results))
	}
	if morePending {
		t.Fatal("morePending = true, want false (no pending spawns)")
	}
}

func TestEmitLoopEvent_NilChannel(t *testing.T) {
	// Should not panic on nil channel.
	ok := emitLoopEvent(nil, "test", struct{}{})
	if ok {
		t.Fatal("emitLoopEvent(nil) = true, want false")
	}
}

func TestEmitLoopEvent_SendsToChannel(t *testing.T) {
	ch := make(chan any, 1)
	ok := emitLoopEvent(ch, "test", "event-data")
	if !ok {
		t.Fatal("emitLoopEvent() = false, want true")
	}
	select {
	case ev := <-ch:
		if ev != "event-data" {
			t.Fatalf("event = %v, want %q", ev, "event-data")
		}
	default:
		t.Fatal("channel should have event")
	}
}

func TestEmitLoopEvent_DropsOnBackpressure(t *testing.T) {
	ch := make(chan any, 1)
	ch <- "blocker"

	ok := emitLoopEvent(ch, "test", "dropped")
	if ok {
		t.Fatal("emitLoopEvent on full channel = true, want false")
	}
}
