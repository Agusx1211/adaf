//go:build integration

package agent

// integration_common_test.go provides a shared test framework for real agent
// integration tests. Each agent defines a thin wrapper (see claude_real_test.go
// etc.) that plugs into this framework, avoiding duplication of test logic.
//
// Tests create real temp directories, run real agent binaries, ask them to
// perform file operations, and verify the results. This tests the full pipeline:
// arg construction -> process spawn -> stream parsing -> result extraction.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

// ---------------------------------------------------------------------------
// Binary discovery
// ---------------------------------------------------------------------------

// findBinary resolves an agent binary by checking (in order):
//  1. Environment variable ADAF_TEST_<AGENT>_BINARY
//  2. exec.LookPath
//  3. Known fallback paths
func findBinary(agentName string) string {
	envKey := "ADAF_TEST_" + strings.ToUpper(agentName) + "_BINARY"
	if p := os.Getenv(envKey); p != "" {
		return p
	}
	if p, err := exec.LookPath(agentName); err == nil {
		return p
	}
	// Fallback to known install locations on this machine.
	fallbacks := map[string][]string{
		"claude":   {"/home/agusx1211/.bun/bin/claude"},
		"codex":    {"/home/agusx1211/.bun/bin/codex"},
		"gemini":   {"/home/agusx1211/.nvm/versions/node/v22.3.0/bin/gemini"},
		"vibe":     {"/home/agusx1211/.local/bin/vibe"},
		"opencode": {"/home/agusx1211/.nvm/versions/node/v22.3.0/bin/opencode"},
	}
	for _, p := range fallbacks[agentName] {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Test workspace
// ---------------------------------------------------------------------------

// testWorkspace manages a temp directory pre-populated with files for a test.
// The directory lives under /tmp/adaf_test_<agent>_<hex> and is cleaned up
// automatically when the test finishes.
type testWorkspace struct {
	Dir string
	t   *testing.T
}

func newTestWorkspace(t *testing.T, agentName string) *testWorkspace {
	t.Helper()
	hexBytes := make([]byte, 8)
	if _, err := rand.Read(hexBytes); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("adaf_test_%s_%s", agentName, hex.EncodeToString(hexBytes)))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	// Initialize a git repository – some agents require it.
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", dir, err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").CombinedOutput(); err != nil {
		t.Fatalf("git config email: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "user.name", "Test").CombinedOutput(); err != nil {
		t.Fatalf("git config name: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit --allow-empty: %v\n%s", err, out)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return &testWorkspace{Dir: dir, t: t}
}

func (ws *testWorkspace) writeFile(name, content string) {
	ws.t.Helper()
	p := filepath.Join(ws.Dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		ws.t.Fatalf("MkdirAll for %s: %v", name, err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		ws.t.Fatalf("WriteFile(%s): %v", name, err)
	}
}

func (ws *testWorkspace) readFile(name string) string {
	ws.t.Helper()
	data, err := os.ReadFile(filepath.Join(ws.Dir, name))
	if err != nil {
		ws.t.Fatalf("ReadFile(%s): %v", name, err)
	}
	return string(data)
}

func (ws *testWorkspace) fileExists(name string) bool {
	_, err := os.Stat(filepath.Join(ws.Dir, name))
	return err == nil
}

// ---------------------------------------------------------------------------
// Agent test suite
// ---------------------------------------------------------------------------

// agentTestSuite defines the shared integration tests that run against every
// agent. Each agent creates one of these in its own test file and calls
// suite.runAll(t).
type agentTestSuite struct {
	// Name is the canonical agent name (e.g. "claude").
	Name string

	// Binary is the absolute path to the agent binary.
	Binary string

	// NewAgent returns a fresh Agent instance.
	NewAgent func() Agent

	// BaseArgs are always passed to the agent (e.g. permission-skip flags).
	BaseArgs []string

	// SupportsResume indicates whether session resume can be tested.
	SupportsResume bool

	// Timeout per individual agent invocation.
	Timeout time.Duration
}

func (s *agentTestSuite) timeout() time.Duration {
	if s.Timeout > 0 {
		return s.Timeout
	}
	return 180 * time.Second
}

func (s *agentTestSuite) skip(t *testing.T) {
	t.Helper()
	if s.Binary == "" {
		t.Skipf("%s binary not found; set ADAF_TEST_%s_BINARY or install it", s.Name, strings.ToUpper(s.Name))
	}
	if _, err := os.Stat(s.Binary); err != nil {
		t.Skipf("%s binary not found at %s: %v", s.Name, s.Binary, err)
	}
}

// run executes the agent with the given prompt in the workspace and returns
// the result.
func (s *agentTestSuite) run(t *testing.T, ws *testWorkspace, prompt string) *Result {
	t.Helper()
	return s.runWithResume(t, ws, prompt, "")
}

// runWithResume executes the agent, optionally resuming a previous session.
func (s *agentTestSuite) runWithResume(t *testing.T, ws *testWorkspace, prompt, resumeSessionID string) *Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	defer cancel()

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: s.Name + "-integration-test"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	result, runErr := s.NewAgent().Run(ctx, Config{
		Command:         s.Binary,
		Args:            s.BaseArgs,
		WorkDir:         ws.Dir,
		Prompt:          prompt,
		ResumeSessionID: resumeSessionID,
	}, rec)
	if runErr != nil {
		t.Fatalf("agent.Run() error: %v", runErr)
	}
	if result == nil {
		t.Fatal("agent.Run() returned nil result")
	}
	return result
}

// runAll runs the full suite of integration tests.
func (s *agentTestSuite) runAll(t *testing.T) {
	s.skip(t)

	t.Run("ModifyFile", s.testModifyFile)
	t.Run("CreateFile", s.testCreateFile)
	t.Run("ReadAndReport", s.testReadAndReport)
	t.Run("NonZeroExitOnBadCommand", s.testNonZeroExit)
	t.Run("ContextCancel", s.testContextCancel)
	t.Run("RecordingCapture", s.testRecordingCapture)

	if s.SupportsResume {
		t.Run("SessionResume", s.testSessionResume)
	}
}

// ---------------------------------------------------------------------------
// Test: file modification
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testModifyFile(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)
	ws.writeFile("target.txt", "The quick brown ALPHA jumps over the lazy ALPHA.")

	result := s.run(t, ws, strings.Join([]string{
		"Read the file target.txt in the current directory.",
		"Replace every occurrence of the word ALPHA with BETA.",
		"Write the modified content back to target.txt.",
		"Do not add any extra text or commentary to the file.",
	}, " "))

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d; stderr:\n%s\nstdout:\n%s", result.ExitCode, result.Error, result.Output)
	}

	got := ws.readFile("target.txt")
	if !strings.Contains(got, "BETA") {
		t.Errorf("target.txt does not contain 'BETA' after modification.\nContent: %q", got)
	}
	if strings.Contains(got, "ALPHA") {
		t.Errorf("target.txt still contains 'ALPHA' after modification.\nContent: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Test: file creation
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testCreateFile(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)

	result := s.run(t, ws, strings.Join([]string{
		"Create a new file called created.txt in the current directory.",
		"The file must contain exactly the text: CREATION_MARKER_12345",
		"Do not create any other files.",
	}, " "))

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d; stderr:\n%s\nstdout:\n%s", result.ExitCode, result.Error, result.Output)
	}

	if !ws.fileExists("created.txt") {
		t.Fatal("created.txt was not created")
	}
	got := ws.readFile("created.txt")
	if !strings.Contains(got, "CREATION_MARKER_12345") {
		t.Errorf("created.txt does not contain expected marker.\nContent: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Test: read file and report contents in output
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testReadAndReport(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)
	ws.writeFile("secret.txt", "SECRET_VALUE_67890")

	result := s.run(t, ws, strings.Join([]string{
		"Read the file secret.txt in the current directory.",
		"Tell me what the file contains. Include the exact contents in your response.",
	}, " "))

	if result.ExitCode != 0 {
		t.Fatalf("exit code %d; stderr:\n%s\nstdout:\n%s", result.ExitCode, result.Error, result.Output)
	}

	if !strings.Contains(result.Output, "SECRET_VALUE_67890") {
		t.Errorf("agent output does not contain the secret value.\nOutput: %q", result.Output)
	}
}

// ---------------------------------------------------------------------------
// Test: context cancellation returns promptly
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testContextCancel(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: s.Name + "-cancel-test"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	done := make(chan struct{})
	start := time.Now()

	go func() {
		defer close(done)
		// Use a prompt that will take a long time to complete.
		s.NewAgent().Run(ctx, Config{
			Command: s.Binary,
			Args:    s.BaseArgs,
			WorkDir: ws.Dir,
			Prompt:  "Write a 10000 word essay about the complete history of computing from 1940 to 2025. Include every major milestone.",
		}, rec)
	}()

	select {
	case <-done:
		// Good.
	case <-time.After(30 * time.Second):
		t.Fatal("agent did not return within 30s after context cancellation — likely hanging")
	}

	elapsed := time.Since(start)
	t.Logf("agent returned in %s after 8s context timeout", elapsed)

	// Should return within a few seconds of the 8s timeout, not 30s.
	if elapsed > 20*time.Second {
		t.Errorf("agent took %s to return after cancellation (expected <20s)", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Test: non-zero exit is captured (not a Run() error)
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testNonZeroExit(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)

	// Ask the agent to run a command that will fail.
	result := s.run(t, ws, strings.Join([]string{
		"Run the shell command: exit 42",
		"Execute it and show the result.",
	}, " "))

	// We don't assert a specific exit code because agents handle failed
	// tool calls differently. The key assertion is that Run() did not
	// return an error (which would mean process management broke) and
	// that we got a result back.
	t.Logf("exit code: %d, output length: %d", result.ExitCode, len(result.Output))
}

// ---------------------------------------------------------------------------
// Test: recording events are captured
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testRecordingCapture(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
	defer cancel()

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := st.Init(store.ProjectConfig{Name: s.Name + "-recording-test"}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	rec := recording.New(1, st)

	_, runErr := s.NewAgent().Run(ctx, Config{
		Command: s.Binary,
		Args:    s.BaseArgs,
		WorkDir: ws.Dir,
		Prompt:  "Say hello",
	}, rec)
	if runErr != nil {
		t.Fatalf("agent.Run() error: %v", runErr)
	}

	events := rec.Events()
	if len(events) == 0 {
		t.Fatal("recorder captured zero events")
	}

	// Check meta events.
	var hasAgent, hasCommand, hasWorkdir, hasStdin bool
	for _, ev := range events {
		switch ev.Type {
		case "meta":
			if strings.HasPrefix(ev.Data, "agent="+s.Name) {
				hasAgent = true
			}
			if strings.HasPrefix(ev.Data, "command=") {
				hasCommand = true
			}
			if strings.HasPrefix(ev.Data, "workdir=") {
				hasWorkdir = true
			}
		case "stdin":
			hasStdin = true
		}
	}

	if !hasAgent {
		t.Errorf("missing agent=%s meta event", s.Name)
	}
	if !hasCommand {
		t.Error("missing command= meta event")
	}
	if !hasWorkdir {
		t.Error("missing workdir= meta event")
	}
	if !hasStdin {
		t.Error("missing stdin recording event")
	}

	t.Logf("recorded %d events total", len(events))
}

// ---------------------------------------------------------------------------
// Test: session resume
// ---------------------------------------------------------------------------

func (s *agentTestSuite) testSessionResume(t *testing.T) {
	ws := newTestWorkspace(t, s.Name)

	// Turn 1: create a file and capture session ID.
	result1 := s.run(t, ws, strings.Join([]string{
		"Create a file called turn1.txt in the current directory",
		"with the exact text: FIRST_TURN_MARKER",
	}, " "))

	if result1.ExitCode != 0 {
		t.Fatalf("turn 1 exit code %d; stderr:\n%s", result1.ExitCode, result1.Error)
	}
	if !ws.fileExists("turn1.txt") {
		t.Fatal("turn1.txt was not created in first turn")
	}

	sessionID := result1.AgentSessionID
	if sessionID == "" {
		t.Skip("agent did not return a session ID; cannot test resume")
	}
	t.Logf("turn 1 session ID: %s", sessionID)

	// Turn 2: resume and create another file.
	result2 := s.runWithResume(t, ws, strings.Join([]string{
		"Now create a second file called turn2.txt in the current directory",
		"with the exact text: SECOND_TURN_MARKER",
	}, " "), sessionID)

	if result2.ExitCode != 0 {
		t.Fatalf("turn 2 exit code %d; stderr:\n%s", result2.ExitCode, result2.Error)
	}
	if !ws.fileExists("turn2.txt") {
		t.Errorf("turn2.txt was not created in resumed turn")
	}

	// Both files should exist.
	if !ws.fileExists("turn1.txt") {
		t.Error("turn1.txt disappeared after resume")
	}
}
