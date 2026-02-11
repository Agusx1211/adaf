package session

import (
	"encoding/json"
	"time"
)

// Wire message types sent over the Unix socket.
const (
	MsgMeta     = "meta"     // Session metadata (sent first on connect)
	MsgStarted  = "started"  // Agent session started
	MsgEvent    = "event"    // Claude stream event
	MsgRaw      = "raw"      // Raw text output (non-Claude agents)
	MsgFinished = "finished" // Single agent session finished
	MsgSpawn    = "spawn"    // Spawn hierarchy update
	MsgDone     = "done"     // Entire agent loop completed
	MsgLive     = "live"     // Marker: replay complete, now streaming live
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
}

// WireStarted signals that a new agent session has begun.
type WireStarted struct {
	SessionID int `json:"session_id"`
}

// WireEvent carries a parsed Claude stream event.
type WireEvent struct {
	Event json.RawMessage `json:"event"`
	Raw   json.RawMessage `json:"raw,omitempty"`
}

// WireRaw carries raw text output for non-Claude agents.
type WireRaw struct {
	Data string `json:"data"`
}

// WireFinished signals that a single agent session completed.
type WireFinished struct {
	SessionID  int           `json:"session_id"`
	ExitCode   int           `json:"exit_code"`
	DurationNS time.Duration `json:"duration_ns"`
	Error      string        `json:"error,omitempty"`
}

// WireSpawnInfo describes a spawn entry.
type WireSpawnInfo struct {
	ID       int    `json:"id"`
	Profile  string `json:"profile"`
	Status   string `json:"status"`
	Question string `json:"question,omitempty"`
}

// WireSpawn carries spawn hierarchy updates.
type WireSpawn struct {
	Spawns []WireSpawnInfo `json:"spawns"`
}

// WireDone signals the entire agent loop has completed.
type WireDone struct {
	Error string `json:"error,omitempty"`
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
