package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/runtui"
	"github.com/agusx1211/adaf/internal/stream"
)

// Client connects to a session daemon and receives events.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	Meta    WireMeta
}

// Connect establishes a connection to the session daemon at the given socket path.
// It reads the initial metadata message and returns a ready-to-use client.
func Connect(socketPath string) (*Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to session: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large events

	c := &Client{
		conn:    conn,
		scanner: scanner,
	}

	// Read the metadata message.
	if !scanner.Scan() {
		conn.Close()
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("reading metadata: %w", err)
		}
		return nil, fmt.Errorf("connection closed before metadata")
	}

	msg, err := DecodeMsg(scanner.Bytes())
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("decoding metadata: %w", err)
	}
	if msg.Type != MsgMeta {
		conn.Close()
		return nil, fmt.Errorf("expected meta message, got %q", msg.Type)
	}

	meta, err := DecodeData[WireMeta](msg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("decoding meta data: %w", err)
	}
	c.Meta = *meta

	return c, nil
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
// The isLive callback is called when replay is complete and live streaming begins.
func (c *Client) StreamEvents(eventCh chan<- any, isLive func()) error {
	defer close(eventCh)
	loopDoneSeen := false

	for c.scanner.Scan() {
		msg, err := DecodeMsg(c.scanner.Bytes())
		if err != nil {
			continue
		}

		switch msg.Type {
		case MsgLive:
			if isLive != nil {
				isLive()
			}

		case MsgStarted:
			data, err := DecodeData[WireStarted](msg)
			if err != nil {
				continue
			}
			eventCh <- runtui.AgentStartedMsg{
				SessionID: data.SessionID,
				TurnHexID: data.TurnHexID,
				StepHexID: data.StepHexID,
				RunHexID:  data.RunHexID,
			}

		case MsgPrompt:
			data, err := DecodeData[WirePrompt](msg)
			if err != nil {
				continue
			}
			eventCh <- runtui.AgentPromptMsg{
				SessionID:      data.SessionID,
				TurnHexID:      data.TurnHexID,
				Prompt:         data.Prompt,
				IsResume:       data.IsResume,
				Truncated:      data.Truncated,
				OriginalLength: data.OriginalLength,
			}

		case MsgEvent:
			data, err := DecodeData[WireEvent](msg)
			if err != nil {
				continue
			}
			var ev stream.ClaudeEvent
			if err := json.Unmarshal(data.Event, &ev); err != nil {
				continue
			}
			eventCh <- runtui.AgentEventMsg{Event: ev, Raw: data.Raw}

		case MsgRaw:
			data, err := DecodeData[WireRaw](msg)
			if err != nil {
				continue
			}
			eventCh <- runtui.AgentRawOutputMsg{Data: data.Data, SessionID: data.SessionID}

		case MsgFinished:
			data, err := DecodeData[WireFinished](msg)
			if err != nil {
				continue
			}
			eventCh <- runtui.AgentFinishedMsg{
				SessionID:     data.SessionID,
				TurnHexID:     data.TurnHexID,
				WaitForSpawns: data.WaitForSpawns,
				Result: &agent.Result{
					ExitCode: data.ExitCode,
					Duration: time.Duration(data.DurationNS),
				},
			}

		case MsgSpawn:
			data, err := DecodeData[WireSpawn](msg)
			if err != nil {
				continue
			}
			spawns := make([]runtui.SpawnInfo, len(data.Spawns))
			for i, s := range data.Spawns {
				spawns[i] = runtui.SpawnInfo{
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
			eventCh <- runtui.SpawnStatusMsg{Spawns: spawns}

		case MsgLoopStepStart:
			data, err := DecodeData[WireLoopStepStart](msg)
			if err != nil {
				continue
			}
			eventCh <- runtui.LoopStepStartMsg{
				RunID:      data.RunID,
				RunHexID:   data.RunHexID,
				StepHexID:  data.StepHexID,
				Cycle:      data.Cycle,
				StepIndex:  data.StepIndex,
				Profile:    data.Profile,
				Turns:      data.Turns,
				TotalSteps: data.TotalSteps,
			}

		case MsgLoopStepEnd:
			data, err := DecodeData[WireLoopStepEnd](msg)
			if err != nil {
				continue
			}
			eventCh <- runtui.LoopStepEndMsg{
				RunID:      data.RunID,
				RunHexID:   data.RunHexID,
				StepHexID:  data.StepHexID,
				Cycle:      data.Cycle,
				StepIndex:  data.StepIndex,
				Profile:    data.Profile,
				TotalSteps: data.TotalSteps,
			}

		case MsgLoopDone:
			data, err := DecodeData[WireLoopDone](msg)
			if err != nil {
				continue
			}
			done := runtui.LoopDoneMsg{
				RunID:    data.RunID,
				RunHexID: data.RunHexID,
				Reason:   data.Reason,
			}
			if data.Error != "" {
				done.Err = fmt.Errorf("%s", data.Error)
			}
			eventCh <- done
			loopDoneSeen = true

		case MsgDone:
			if loopDoneSeen {
				return nil
			}
			data, err := DecodeData[WireDone](msg)
			if err != nil {
				eventCh <- runtui.AgentLoopDoneMsg{}
				return nil
			}
			msg := runtui.AgentLoopDoneMsg{}
			if data.Error != "" {
				msg.Err = fmt.Errorf("%s", data.Error)
			}
			eventCh <- msg
			return nil
		}
	}

	if err := c.scanner.Err(); err != nil {
		return err
	}

	// Connection closed without a done message — daemon may have died.
	eventCh <- runtui.AgentLoopDoneMsg{
		Err: fmt.Errorf("connection to session daemon lost"),
	}
	return nil
}

// Cancel sends a cancel request to the daemon.
func (c *Client) Cancel() error {
	_, err := fmt.Fprintf(c.conn, "%s\n", CtrlCancel)
	return err
}

// Close disconnects from the daemon without cancelling the agent.
func (c *Client) Close() error {
	return c.conn.Close()
}
