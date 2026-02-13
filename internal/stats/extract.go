package stats

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agusx1211/adaf/internal/store"
	"github.com/agusx1211/adaf/internal/stream"
)

// SessionMetrics holds metrics extracted from a session's recording.
type SessionMetrics struct {
	TotalCostUSD float64
	InputTokens  int
	OutputTokens int
	NumTurns     int
	DurationSecs int
	ToolCalls    map[string]int // tool_name -> invocation count
	Success      bool
}

// ExtractFromRecording reads the events.jsonl file for a turn and
// parses claude_stream events to extract metrics.
func ExtractFromRecording(st *store.Store, turnID int) (*SessionMetrics, error) {
	eventsPath, err := findEventsFile(st, turnID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("opening events file: %w", err)
	}
	defer f.Close()

	m := &SessionMetrics{
		ToolCalls: make(map[string]int),
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		var event store.RecordingEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		switch event.Type {
		case "claude_stream":
			processStreamEvent(m, event.Data)
		case "meta":
			processMetaEvent(m, event.Data)
		}
	}

	return m, scanner.Err()
}

// findEventsFile locates the events.jsonl for a given turn ID.
func findEventsFile(st *store.Store, turnID int) (string, error) {
	for _, dir := range st.RecordsDirs() {
		path := filepath.Join(dir, fmt.Sprintf("%d", turnID), "events.jsonl")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("events.jsonl not found for turn %d", turnID)
}

// processStreamEvent parses a claude_stream event and extracts metrics.
func processStreamEvent(m *SessionMetrics, data string) {
	var ev stream.ClaudeEvent
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return
	}

	switch ev.Type {
	case "result":
		if ev.TotalCostUSD > 0 {
			m.TotalCostUSD = ev.TotalCostUSD
		}
		if ev.NumTurns > 0 {
			m.NumTurns = ev.NumTurns
		}
		if ev.DurationMS > 0 {
			m.DurationSecs = int(ev.DurationMS / 1000)
		}
		if ev.Usage != nil {
			m.InputTokens = ev.Usage.InputTokens
			m.OutputTokens = ev.Usage.OutputTokens
		}

	case "assistant":
		if ev.AssistantMessage != nil {
			for _, block := range ev.AssistantMessage.Content {
				if block.Type == "tool_use" && block.Name != "" {
					m.ToolCalls[block.Name]++
				}
			}
		}
	}
}

// processMetaEvent checks for exit_code meta events to determine success.
func processMetaEvent(m *SessionMetrics, data string) {
	if strings.HasPrefix(data, "exit_code=") {
		code := strings.TrimPrefix(data, "exit_code=")
		m.Success = code == "0"
	}
}
