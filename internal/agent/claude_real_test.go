//go:build integration

package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/recording"
	"github.com/agusx1211/adaf/internal/store"
)

func TestClaudeRealIntegration(t *testing.T) {
	suite := &agentTestSuite{
		Name:           "claude",
		Binary:         findBinary("claude"),
		NewAgent:       func() Agent { return NewClaudeAgent() },
		BaseArgs:       []string{"--dangerously-skip-permissions", "--max-turns", "3"},
		SupportsResume: true,
		Timeout:        180 * time.Second,
	}

	suite.runAll(t)

	// Claude-specific test: verify that NDJSON stream events are recorded
	// and that the system/init event captures a session ID.
	t.Run("TestStreamEventsRecorded", func(t *testing.T) {
		suite.skip(t)

		ws := newTestWorkspace(t, "claude")

		ctx, cancel := context.WithTimeout(context.Background(), suite.timeout())
		defer cancel()

		st, err := store.New(t.TempDir())
		if err != nil {
			t.Fatalf("store.New: %v", err)
		}
		if err := st.Init(store.ProjectConfig{Name: "claude-stream-test"}); err != nil {
			t.Fatalf("store.Init: %v", err)
		}
		rec := recording.New(1, st)

		result, runErr := suite.NewAgent().Run(ctx, Config{
			Command: suite.Binary,
			Args:    suite.BaseArgs,
			WorkDir: ws.Dir,
			Prompt:  "Say hello",
		}, rec)
		if runErr != nil {
			t.Fatalf("agent.Run() error: %v", runErr)
		}
		if result == nil {
			t.Fatal("agent.Run() returned nil result")
		}

		// Verify claude_stream events were recorded.
		events := rec.Events()
		var streamCount int
		for _, ev := range events {
			if ev.Type == "claude_stream" {
				streamCount++
			}
		}
		t.Logf("claude_stream events recorded: %d", streamCount)
		if streamCount == 0 {
			t.Error("no claude_stream events recorded; expected NDJSON stream events from Claude output")
		}

		// Verify that at least one stream event contains system/init data
		// (indicating the session was properly initialized).
		var hasInitEvent bool
		for _, ev := range events {
			if ev.Type == "claude_stream" && strings.Contains(ev.Data, "\"type\":\"system\"") {
				hasInitEvent = true
				break
			}
		}
		if !hasInitEvent {
			t.Error("no system init event found in claude_stream events")
		}

		// Verify the result has a non-empty session ID, which is extracted
		// from the init event during stream parsing.
		if result.AgentSessionID == "" {
			t.Error("result.AgentSessionID is empty; expected a session ID from the init event")
		} else {
			t.Logf("agent session ID: %s", result.AgentSessionID)
		}
	})
}
