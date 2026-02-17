package session

import (
	"encoding/json"
	"time"

	"github.com/agusx1211/adaf/internal/config"
)

// Wire message types sent over the Unix socket.
const (
	MsgMeta          = "meta"            // Session metadata (sent first on connect)
	MsgSnapshot      = "snapshot"        // Current daemon state + bounded recent output
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
	MsgLive          = "live"            // Marker: snapshot sent, now streaming live
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

// WireSnapshot captures current daemon state sent once on client connect.
type WireSnapshot struct {
	Loop    WireSnapshotLoop     `json:"loop,omitempty"`
	Session *WireSnapshotSession `json:"session,omitempty"`
	Spawns  []WireSpawnInfo      `json:"spawns,omitempty"`
	Recent  []WireMsg            `json:"recent,omitempty"`
}

// WireSnapshotLoop is the current loop/step state for reconnects.
type WireSnapshotLoop struct {
	RunID     int    `json:"run_id,omitempty"`
	RunHexID  string `json:"run_hex_id,omitempty"`
	StepHexID string `json:"step_hex_id,omitempty"`
	// Cycle/StepIndex are intentionally always present so zero-based values are
	// preserved across reconnect snapshots.
	Cycle      int    `json:"cycle"`
	StepIndex  int    `json:"step_index"`
	Profile    string `json:"profile,omitempty"`
	TotalSteps int    `json:"total_steps,omitempty"`
}

// WireSnapshotSession is the current turn/session state for reconnects.
// NOTE: keep fields value-only; cloneWireSnapshot relies on shallow copy safety.
type WireSnapshotSession struct {
	SessionID    int       `json:"session_id"`
	TurnHexID    string    `json:"turn_hex_id,omitempty"`
	Agent        string    `json:"agent,omitempty"`
	Profile      string    `json:"profile,omitempty"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
	NumTurns     int       `json:"num_turns,omitempty"`
	Status       string    `json:"status,omitempty"`
	Action       string    `json:"action,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	EndedAt      time.Time `json:"ended_at,omitempty"`
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
	Event   json.RawMessage `json:"event"`
	Raw     json.RawMessage `json:"raw,omitempty"`
	SpawnID int             `json:"spawn_id,omitempty"`
	TurnID  int             `json:"turn_id,omitempty"`
}

// WireRaw carries raw text output for non-Claude agents.
type WireRaw struct {
	Data      string `json:"data"`
	SessionID int    `json:"session_id,omitempty"`
	SpawnID   int    `json:"spawn_id,omitempty"`
}

// WireFinished signals that a single agent session completed.
type WireFinished struct {
	SessionID     int    `json:"session_id"`
	TurnHexID     string `json:"turn_hex_id,omitempty"`
	WaitForSpawns bool   `json:"wait_for_spawns,omitempty"`
	ExitCode      int    `json:"exit_code"`
	DurationNS    int64  `json:"duration_ns"`
	Error         string `json:"error,omitempty"`
}

// WireSpawnInfo describes a spawn entry.
type WireSpawnInfo struct {
	ID            int    `json:"id"`
	ParentTurnID  int    `json:"parent_turn_id,omitempty"`
	ParentSpawnID int    `json:"parent_spawn_id,omitempty"`
	ChildTurnID   int    `json:"child_turn_id,omitempty"`
	Profile       string `json:"profile"`
	Position      string `json:"position,omitempty"`
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
	Action    string                `json:"action"`
	Spawn     *WireControlSpawn     `json:"spawn,omitempty"`
	Wait      *WireControlWait      `json:"wait,omitempty"`
	Interrupt *WireControlInterrupt `json:"interrupt,omitempty"`
}

// WireControlSpawn carries a spawn request executed by the daemon.
type WireControlSpawn struct {
	ParentTurnID         int                      `json:"parent_turn_id"`
	ParentProfile        string                   `json:"parent_profile"`
	ParentPosition       string                   `json:"parent_position,omitempty"`
	ChildProfile         string                   `json:"child_profile"`
	ChildPosition        string                   `json:"child_position,omitempty"`
	ChildRole            string                   `json:"child_role,omitempty"`
	PlanID               string                   `json:"plan_id,omitempty"`
	Task                 string                   `json:"task"`
	IssueIDs             []int                    `json:"issue_ids,omitempty"`
	WorkspaceFromSpawnID int                      `json:"workspace_from_spawn_id,omitempty"`
	ReadOnly             bool                     `json:"read_only,omitempty"`
	Wait                 bool                     `json:"wait,omitempty"`
	Delegation           *config.DelegationConfig `json:"delegation,omitempty"`
}

// WireControlWait carries a wait-for-spawns signal request.
type WireControlWait struct {
	TurnID int `json:"turn_id"`
}

// WireControlInterrupt carries an interrupt request for a running spawn.
type WireControlInterrupt struct {
	SpawnID int    `json:"spawn_id"`
	Message string `json:"message"`
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
