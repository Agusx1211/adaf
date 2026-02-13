//go:build integration

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestOpencodeRealIntegration(t *testing.T) {
	suite := &agentTestSuite{
		Name:           "opencode",
		Binary:         findBinary("opencode"),
		NewAgent:       func() Agent { return NewOpencodeAgent() },
		BaseArgs:       []string{},
		SupportsResume: true,
		Timeout:        180 * time.Second,
	}

	suite.runAll(t)

	// OpenCode-specific: verify stream capture works with --format json.
	t.Run("TestOpencodeStreamOutput", func(t *testing.T) {
		suite.skip(t)

		ws := newTestWorkspace(t, "opencode")

		result := suite.run(t, ws, "Say exactly: OPENCODE_OUTPUT_CHECK")

		if result.ExitCode != 0 {
			t.Fatalf("exit code %d; stderr:\n%s\nstdout:\n%s", result.ExitCode, result.Error, result.Output)
		}

		if strings.TrimSpace(result.Output) == "" {
			t.Fatal("result.Output is empty; stream capture failed for opencode")
		}

		t.Logf("output length: %d bytes", len(result.Output))

		if !strings.Contains(result.Output, "OPENCODE_OUTPUT_CHECK") {
			t.Errorf("output does not contain expected marker.\nOutput: %q", result.Output)
		}

		if result.AgentSessionID == "" {
			t.Error("AgentSessionID is empty; session ID capture failed for opencode")
		} else {
			t.Logf("captured session ID: %s", result.AgentSessionID)
		}
	})
}
