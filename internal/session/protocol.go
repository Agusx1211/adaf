package session

import (
	"encoding/json"
	"time"

	"github.com/agusx1211/adaf/internal/config"
)

// Wire message types sent over the Unix socket.
const (
	MsgMeta          = "meta"            // Session metadata (sent first on connect)
	MsgStarted       = "started"         // Agent session started
	MsgPrompt        = "prompt"          // Agent turn prompt payload
	MsgEvent         = "event"           // Claude stream event
	MsgRaw           = "raw"             // Raw text output (non-Claude agents)
	MsgFinished      = "finished"        // Single agent session finished
	MsgSpawn         = "spawn"           // Spawn hierarchy update
	MsgControl       = "control"         // Client -> daemon control request
	MsgControlResult = "control_result"  // Daemon -> client control response
	MsgLoopStepStart = "loop_step_start" // Loop step started
	MsgLoopStepEnd   = "loop_step_end"   // Loop step ended
	MsgLoopDone      = "loop_done"       // Loop finished
	MsgDone          = "done"            // Entire agent loop completed
	MsgLive          = "live"            // Marker: replay complete, now streaming live
)

// Client-to-daemon control messages.
const (
	CtrlCancel = "cancel" // Request agent cancellation
)

// WireMsg is the envelope for all messages sent over the session socket.
// Each message is a single JSON line terminated by newline.
type WireMsg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// WireMeta is sent as the first message to a connecting client.
type WireMeta struct {
	SessionID   int    `json:"session_id"`
	ProfileName string `json:"profile"`
	AgentName   string `json:"agent"`
	ProjectName string `json:"project"`
	LoopName    string `json:"loop_name,omitempty"`
	LoopSteps   int    `json:"loop_steps,omitempty"`
}

// WireStarted signals that a new agent session has begun.
type WireStarted struct {
	SessionID int    `json:"session_id"`
	TurnHexID string `json:"turn_hex_id,omitempty"`
	StepHexID string `json:"step_hex_id,omitempty"`
	RunHexID  string `json:"run_hex_id,omitempty"`
}

// WirePrompt carries the prompt payload for a turn.
type WirePrompt struct {
	SessionID      int    `json:"session_id"`
	TurnHexID      string `json:"turn_hex_id,omitempty"`
	Prompt         string `json:"prompt"`
	IsResume       bool   `json:"is_resume,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	OriginalLength int    `json:"original_length,omitempty"`
}

// WireEvent carries a parsed Claude stream event.
type WireEvent struct {
	Event json.RawMessage `json:"event"`
	Raw   json.RawMessage `json:"raw,omitempty"`
}

// WireRaw carries raw text output for non-Claude agents.
type WireRaw struct {
	Data      string `json:"data"`
	SessionID int    `json:"session_id,omitempty"`
}

// WireFinished signals that a single agent session completed.
type WireFinished struct {
	SessionID     int           `json:"session_id"`
	TurnHexID     string        `json:"turn_hex_id,omitempty"`
	WaitForSpawns bool          `json:"wait_for_spawns,omitempty"`
	ExitCode      int           `json:"exit_code"`
	DurationNS    time.Duration `json:"duration_ns"`
	Error         string        `json:"error,omitempty"`
}

// WireSpawnInfo describes a spawn entry.
type WireSpawnInfo struct {
	ID            int    `json:"id"`
	ParentTurnID  int    `json:"parent_turn_id,omitempty"`
	ParentSpawnID int    `json:"parent_spawn_id,omitempty"`
	ChildTurnID   int    `json:"child_turn_id,omitempty"`
	Profile       string `json:"profile"`
	Role          string `json:"role,omitempty"`
	Status        string `json:"status"`
	Question      string `json:"question,omitempty"`
}

// WireSpawn carries spawn hierarchy updates.
type WireSpawn struct {
	Spawns []WireSpawnInfo `json:"spawns"`
}

// WireLoopStepStart signals a loop step start.
type WireLoopStepStart struct {
	RunID      int    `json:"run_id"`
	RunHexID   string `json:"run_hex_id,omitempty"`
	StepHexID  string `json:"step_hex_id,omitempty"`
	Cycle      int    `json:"cycle"`
	StepIndex  int    `json:"step_index"`
	Profile    string `json:"profile"`
	Turns      int    `json:"turns"`
	TotalSteps int    `json:"total_steps,omitempty"`
}

// WireLoopStepEnd signals a loop step end.
type WireLoopStepEnd struct {
	RunID      int    `json:"run_id"`
	RunHexID   string `json:"run_hex_id,omitempty"`
	StepHexID  string `json:"step_hex_id,omitempty"`
	Cycle      int    `json:"cycle"`
	StepIndex  int    `json:"step_index"`
	Profile    string `json:"profile"`
	TotalSteps int    `json:"total_steps,omitempty"`
}

// WireLoopDone signals the loop completion state.
type WireLoopDone struct {
	RunID    int    `json:"run_id,omitempty"`
	RunHexID string `json:"run_hex_id,omitempty"`
	Reason   string `json:"reason,omitempty"` // "stopped", "cancelled", "error"
	Error    string `json:"error,omitempty"`
}

// WireDone signals the entire agent loop has completed.
type WireDone struct {
	Error string `json:"error,omitempty"`
}

// WireControl is a client -> daemon control request.
type WireControl struct {
	Action string            `json:"action"`
	Spawn  *WireControlSpawn `json:"spawn,omitempty"`
}

// WireControlSpawn carries a spawn request executed by the daemon.
type WireControlSpawn struct {
	ParentTurnID  int                      `json:"parent_turn_id"`
	ParentProfile string                   `json:"parent_profile"`
	ChildProfile  string                   `json:"child_profile"`
	ChildRole     string                   `json:"child_role,omitempty"`
	PlanID        string                   `json:"plan_id,omitempty"`
	Task          string                   `json:"task"`
	ReadOnly      bool                     `json:"read_only,omitempty"`
	Wait          bool                     `json:"wait,omitempty"`
	Delegation    *config.DelegationConfig `json:"delegation,omitempty"`
}

// WireControlResult is a daemon -> client reply for a control request.
type WireControlResult struct {
	Action   string `json:"action"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	SpawnID  int    `json:"spawn_id,omitempty"`
	Status   string `json:"status,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Result   string `json:"result,omitempty"`
}

// EncodeMsg creates a JSON line from a message type and payload.
func EncodeMsg(msgType string, payload any) ([]byte, error) {
	var dataBytes json.RawMessage
	if payload != nil {
		var err error
		dataBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	msg := WireMsg{Type: msgType, Data: dataBytes}
	line, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

// DecodeMsg parses a JSON line into a WireMsg.
func DecodeMsg(line []byte) (*WireMsg, error) {
	var msg WireMsg
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// DecodeData unmarshals the Data field of a WireMsg into the target struct.
func DecodeData[T any](msg *WireMsg) (*T, error) {
	var v T
	if err := json.Unmarshal(msg.Data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
