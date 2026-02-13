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

func TestGeminiRealIntegration(t *testing.T) {
	suite := &agentTestSuite{
		Name:           "gemini",
		Binary:         findBinary("gemini"),
		NewAgent:       func() Agent { return NewGeminiAgent() },
		BaseArgs:       []string{"-y"},
		SupportsResume: true,
		Timeout:        180 * time.Second,
	}

	suite.runAll(t)

	// Gemini-specific: verify stream events and session ID.
	t.Run("TestGeminiStreamEvents", func(t *testing.T) {
		suite.skip(t)

		ws := newTestWorkspace(t, "gemini")

		ctx, cancel := context.WithTimeout(context.Background(), suite.timeout())
		defer cancel()

		st, err := store.New(t.TempDir())
		if err != nil {
			t.Fatalf("store.New: %v", err)
		}
		if err := st.Init(store.ProjectConfig{Name: "gemini-stream-test"}); err != nil {
			t.Fatalf("store.Init: %v", err)
		}
		rec := recording.New(1, st)

		result, runErr := suite.NewAgent().Run(ctx, Config{
			Command: suite.Binary,
			Args:    suite.BaseArgs,
			WorkDir: ws.Dir,
			Prompt:  "Say hello world",
		}, rec)
		if runErr != nil {
			t.Fatalf("agent.Run() error: %v", runErr)
		}
		if result == nil {
			t.Fatal("agent.Run() returned nil result")
		}

		// Verify that Gemini NDJSON was translated to claude_stream events.
		events := rec.Events()
		var streamCount int
		for _, ev := range events {
			if ev.Type == "claude_stream" {
				streamCount++
			}
		}
		if streamCount == 0 {
			t.Error("no claude_stream events recorded; Gemini NDJSON should be translated to claude_stream format")
		}
		t.Logf("recorded %d claude_stream events", streamCount)

		// Log a sample of stream events for debugging.
		logged := 0
		for _, ev := range events {
			if ev.Type == "claude_stream" && logged < 5 {
				data := ev.Data
				if len(data) > 200 {
					data = data[:200] + "..."
				}
				t.Logf("  stream event: %s", data)
				logged++
			}
		}

		// Verify AgentSessionID is non-empty.
		if strings.TrimSpace(result.AgentSessionID) == "" {
			t.Error("result.AgentSessionID is empty; expected Gemini to return a session ID")
		} else {
			t.Logf("AgentSessionID: %s", result.AgentSessionID)
		}
	})
}
