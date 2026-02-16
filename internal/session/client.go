package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/events"
	"github.com/agusx1211/adaf/internal/stream"
)

// Client connects to a session daemon and receives events.
type Client struct {
	ws                *websocket.Conn
	ctx               context.Context
	cancel            context.CancelFunc
	socketPath        string
	Meta              WireMeta
	unknownTypeLogged map[string]struct{}
}

// Connect establishes a WebSocket connection to the session daemon at the given socket path.
// It reads the initial metadata message and returns a ready-to-use client.
func Connect(socketPath string) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())

	ws, meta, err := dialAndHandshake(ctx, socketPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connecting to session: %w", err)
	}

	c := &Client{
		ws:                ws,
		ctx:               ctx,
		cancel:            cancel,
		socketPath:        socketPath,
		Meta:              *meta,
		unknownTypeLogged: make(map[string]struct{}),
	}

	return c, nil
}

func dialAndHandshake(ctx context.Context, socketPath string) (*websocket.Conn, *WireMeta, error) {
	// Dial WebSocket over Unix socket.
	ws, _, err := websocket.Dial(ctx, "ws://localhost/", &websocket.DialOptions{
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
				},
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	// Set a generous read limit for large snapshot messages.
	ws.SetReadLimit(4 * 1024 * 1024)

	// Read the metadata message.
	_, data, err := ws.Read(ctx)
	if err != nil {
		ws.Close(websocket.StatusNormalClosure, "")
		return nil, nil, fmt.Errorf("reading metadata: %w", err)
	}

	msg, err := DecodeMsg(data)
	if err != nil {
		ws.Close(websocket.StatusNormalClosure, "")
		return nil, nil, fmt.Errorf("decoding metadata: %w", err)
	}
	if msg.Type != MsgMeta {
		ws.Close(websocket.StatusNormalClosure, "")
		return nil, nil, fmt.Errorf("expected meta message, got %q", msg.Type)
	}

	meta, err := DecodeData[WireMeta](msg)
	if err != nil {
		ws.Close(websocket.StatusNormalClosure, "")
		return nil, nil, fmt.Errorf("decoding meta data: %w", err)
	}

	return ws, meta, nil
}

// ConnectToSession connects to a session by ID, looking up the socket path.
func ConnectToSession(sessionID int) (*Client, error) {
	meta, err := LoadMeta(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session %d not found: %w", sessionID, err)
	}
	if !IsActiveStatus(meta.Status) {
		return nil, fmt.Errorf("session %d is not running (status: %s)", sessionID, meta.Status)
	}

	return Connect(SocketPath(sessionID))
}

// StreamEvents reads events from the daemon and sends them on the provided channel.
// It blocks until the connection is closed or an error occurs.
// The isLive callback is called when the snapshot has been delivered and live
// streaming begins.
func (c *Client) StreamEvents(eventCh chan<- any, isLive func()) error {
	defer close(eventCh)
	loopDoneSeen := false

	for {
		_, data, err := c.ws.Read(c.ctx)
		if err != nil {
			// Check if context was cancelled (intentional close).
			if c.ctx.Err() != nil {
				return nil
			}

			// If it's a normal closure, we're done.
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return nil
			}

			// Connection lost — attempt reconnect.
			debug.LogKV("session.client", "connection lost, attempting reconnect", "error", err)
			if recErr := c.reconnect(); recErr != nil {
				debug.LogKV("session.client", "reconnection failed", "error", recErr)
				// Connection closed without a done message — daemon may have died.
				eventCh <- events.AgentLoopDoneMsg{
					Err: fmt.Errorf("connection to session daemon lost: %w", err),
				}
				return nil
			}
			debug.LogKV("session.client", "reconnected")
			continue
		}

		msg, err := DecodeMsg(data)
		if err != nil {
			continue
		}

		switch msg.Type {
		case MsgLive:
			if isLive != nil {
				isLive()
			}
			continue

		case MsgSnapshot:
			snapData, err := DecodeData[WireSnapshot](msg)
			if err != nil {
				continue
			}
			snapshot := events.SessionSnapshotMsg{
				Loop: events.SessionLoopSnapshot{
					RunID:      snapData.Loop.RunID,
					RunHexID:   snapData.Loop.RunHexID,
					StepHexID:  snapData.Loop.StepHexID,
					Cycle:      snapData.Loop.Cycle,
					StepIndex:  snapData.Loop.StepIndex,
					Profile:    snapData.Loop.Profile,
					TotalSteps: snapData.Loop.TotalSteps,
				},
			}
			if snapData.Session != nil {
				snapshot.Session = &events.SessionTurnSnapshot{
					SessionID:    snapData.Session.SessionID,
					TurnHexID:    snapData.Session.TurnHexID,
					Agent:        snapData.Session.Agent,
					Profile:      snapData.Session.Profile,
					Model:        snapData.Session.Model,
					InputTokens:  snapData.Session.InputTokens,
					OutputTokens: snapData.Session.OutputTokens,
					CostUSD:      snapData.Session.CostUSD,
					NumTurns:     snapData.Session.NumTurns,
					Status:       snapData.Session.Status,
					Action:       snapData.Session.Action,
					StartedAt:    snapData.Session.StartedAt,
					EndedAt:      snapData.Session.EndedAt,
				}
			}
			if len(snapData.Spawns) > 0 {
				spawns := make([]events.SpawnInfo, len(snapData.Spawns))
				for i, s := range snapData.Spawns {
					spawns[i] = events.SpawnInfo{
						ID:            s.ID,
						ParentTurnID:  s.ParentTurnID,
						ParentSpawnID: s.ParentSpawnID,
						ChildTurnID:   s.ChildTurnID,
						Profile:       s.Profile,
						Role:          s.Role,
						Status:        s.Status,
						Question:      s.Question,
					}
				}
				snapshot.Spawns = spawns
			}
			eventCh <- snapshot

			for i := range snapData.Recent {
				if !isSnapshotRecentType(snapData.Recent[i].Type) {
					c.logUnknownWireType("unsupported snapshot.recent type", snapData.Recent[i].Type)
					continue
				}
				if done := c.forwardEventMsg(eventCh, &snapData.Recent[i], &loopDoneSeen); done {
					return nil
				}
			}
			continue
		}

		if done := c.forwardEventMsg(eventCh, msg, &loopDoneSeen); done {
			return nil
		}
	}
}

func (c *Client) reconnect() error {
	backoff := 100 * time.Millisecond
	maxBackoff := 5 * time.Second
	totalTimeout := 30 * time.Second
	start := time.Now()

	for {
		if time.Since(start) > totalTimeout {
			return fmt.Errorf("reconnection timed out after %v", totalTimeout)
		}

		debug.LogKV("session.client", "reconnecting", "socket", c.socketPath, "backoff", backoff)

		ws, meta, err := dialAndHandshake(c.ctx, c.socketPath)
		if err == nil {
			c.ws = ws
			c.Meta = *meta
			return nil
		}

		debug.LogKV("session.client", "reconnect failed", "error", err)

		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *Client) forwardEventMsg(eventCh chan<- any, msg *WireMsg, loopDoneSeen *bool) bool {
	switch msg.Type {
	case MsgStarted:
		data, err := DecodeData[WireStarted](msg)
		if err != nil {
			return false
		}
		eventCh <- events.AgentStartedMsg{
			SessionID: data.SessionID,
			TurnHexID: data.TurnHexID,
			StepHexID: data.StepHexID,
			RunHexID:  data.RunHexID,
		}
		return false

	case MsgPrompt:
		data, err := DecodeData[WirePrompt](msg)
		if err != nil {
			return false
		}
		eventCh <- events.AgentPromptMsg{
			SessionID:      data.SessionID,
			TurnHexID:      data.TurnHexID,
			Prompt:         data.Prompt,
			IsResume:       data.IsResume,
			Truncated:      data.Truncated,
			OriginalLength: data.OriginalLength,
		}
		return false

	case MsgEvent:
		data, err := DecodeData[WireEvent](msg)
		if err != nil {
			return false
		}
		var ev stream.ClaudeEvent
		if err := json.Unmarshal(data.Event, &ev); err != nil {
			return false
		}
		eventCh <- events.AgentEventMsg{Event: ev, Raw: data.Raw}
		return false

	case MsgRaw:
		data, err := DecodeData[WireRaw](msg)
		if err != nil {
			return false
		}
		eventCh <- events.AgentRawOutputMsg{Data: data.Data, SessionID: data.SessionID}
		return false

	case MsgFinished:
		data, err := DecodeData[WireFinished](msg)
		if err != nil {
			return false
		}
		finished := events.AgentFinishedMsg{
			SessionID:     data.SessionID,
			TurnHexID:     data.TurnHexID,
			WaitForSpawns: data.WaitForSpawns,
			Result: &agent.Result{
				ExitCode: data.ExitCode,
				Duration: time.Duration(data.DurationNS),
			},
		}
		if data.Error != "" {
			finished.Err = fmt.Errorf("%s", data.Error)
		}
		eventCh <- finished
		return false

	case MsgSpawn:
		data, err := DecodeData[WireSpawn](msg)
		if err != nil {
			return false
		}
		spawns := make([]events.SpawnInfo, len(data.Spawns))
		for i, s := range data.Spawns {
			spawns[i] = events.SpawnInfo{
				ID:            s.ID,
				ParentTurnID:  s.ParentTurnID,
				ParentSpawnID: s.ParentSpawnID,
				ChildTurnID:   s.ChildTurnID,
				Profile:       s.Profile,
				Role:          s.Role,
				Status:        s.Status,
				Question:      s.Question,
			}
		}
		eventCh <- events.SpawnStatusMsg{Spawns: spawns}
		return false

	case MsgLoopStepStart:
		data, err := DecodeData[WireLoopStepStart](msg)
		if err != nil {
			return false
		}
		eventCh <- events.LoopStepStartMsg{
			RunID:      data.RunID,
			RunHexID:   data.RunHexID,
			StepHexID:  data.StepHexID,
			Cycle:      data.Cycle,
			StepIndex:  data.StepIndex,
			Profile:    data.Profile,
			Turns:      data.Turns,
			TotalSteps: data.TotalSteps,
		}
		return false

	case MsgLoopStepEnd:
		data, err := DecodeData[WireLoopStepEnd](msg)
		if err != nil {
			return false
		}
		eventCh <- events.LoopStepEndMsg{
			RunID:      data.RunID,
			RunHexID:   data.RunHexID,
			StepHexID:  data.StepHexID,
			Cycle:      data.Cycle,
			StepIndex:  data.StepIndex,
			Profile:    data.Profile,
			TotalSteps: data.TotalSteps,
		}
		return false

	case MsgLoopDone:
		data, err := DecodeData[WireLoopDone](msg)
		if err != nil {
			return false
		}
		done := events.LoopDoneMsg{
			RunID:    data.RunID,
			RunHexID: data.RunHexID,
			Reason:   data.Reason,
		}
		if data.Error != "" {
			done.Err = fmt.Errorf("%s", data.Error)
		}
		eventCh <- done
		*loopDoneSeen = true
		return false

	case MsgDone:
		if *loopDoneSeen {
			return true
		}
		data, err := DecodeData[WireDone](msg)
		if err != nil {
			eventCh <- events.AgentLoopDoneMsg{}
			return true
		}
		done := events.AgentLoopDoneMsg{}
		if data.Error != "" {
			done.Err = fmt.Errorf("%s", data.Error)
		}
		eventCh <- done
		return true
	}

	c.logUnknownWireType("ignoring unknown wire message type", msg.Type)
	return false
}

func isSnapshotRecentType(msgType string) bool {
	switch msgType {
	case MsgPrompt, MsgEvent, MsgRaw:
		return true
	default:
		return false
	}
}

func (c *Client) logUnknownWireType(prefix, msgType string) {
	if msgType == "" {
		msgType = "<empty>"
	}
	if c.unknownTypeLogged == nil {
		c.unknownTypeLogged = make(map[string]struct{})
	}
	key := prefix + "|" + msgType
	if _, seen := c.unknownTypeLogged[key]; seen {
		return
	}
	c.unknownTypeLogged[key] = struct{}{}
	fmt.Fprintf(os.Stderr, "session client: %s: %q\n", prefix, msgType)
}

// Cancel sends a cancel request to the daemon.
func (c *Client) Cancel() error {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	return c.ws.Write(ctx, websocket.MessageText, []byte(CtrlCancel))
}

// Close disconnects from the daemon without cancelling the agent.
func (c *Client) Close() error {
	c.cancel()
	return c.ws.Close(websocket.StatusNormalClosure, "detach")
}
