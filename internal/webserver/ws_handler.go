package webserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/agusx1211/adaf/internal/events"
	"github.com/agusx1211/adaf/internal/session"
)

type wsEnvelope struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

func (srv *Server) handleSessionWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID, err := parsePathID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer ws.CloseNow()

	ctx := r.Context()

	client, err := session.ConnectToSession(sessionID)
	if err != nil {
		data, _ := json.Marshal(wsEnvelope{Type: "error", Data: errorResponse{Error: err.Error()}})
		_ = ws.Write(ctx, websocket.MessageText, data)
		ws.Close(websocket.StatusInternalError, err.Error())
		return
	}
	defer client.Close()

	eventCh := make(chan any, 256)
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.StreamEvents(eventCh, nil)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventCh:
			if !ok {
				if streamErr := <-errCh; streamErr != nil {
					data, _ := json.Marshal(wsEnvelope{Type: "error", Data: errorResponse{Error: streamErr.Error()}})
					_ = ws.Write(ctx, websocket.MessageText, data)
				}
				ws.Close(websocket.StatusNormalClosure, "stream ended")
				return
			}

			msg := toWSEnvelope(event)
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			writeCtx, writeCancel := context.WithTimeout(ctx, 15*time.Second)
			if err := ws.Write(writeCtx, websocket.MessageText, data); err != nil {
				writeCancel()
				return
			}
			writeCancel()
		}
	}
}

func toWSEnvelope(event any) wsEnvelope {
	switch ev := event.(type) {
	case events.SessionSnapshotMsg:
		msg := session.WireSnapshot{
			Loop: session.WireSnapshotLoop{
				RunID:      ev.Loop.RunID,
				RunHexID:   ev.Loop.RunHexID,
				StepHexID:  ev.Loop.StepHexID,
				Cycle:      ev.Loop.Cycle,
				StepIndex:  ev.Loop.StepIndex,
				Profile:    ev.Loop.Profile,
				TotalSteps: ev.Loop.TotalSteps,
			},
		}
		if ev.Session != nil {
			msg.Session = &session.WireSnapshotSession{
				SessionID:    ev.Session.SessionID,
				TurnHexID:    ev.Session.TurnHexID,
				Agent:        ev.Session.Agent,
				Profile:      ev.Session.Profile,
				Model:        ev.Session.Model,
				InputTokens:  ev.Session.InputTokens,
				OutputTokens: ev.Session.OutputTokens,
				CostUSD:      ev.Session.CostUSD,
				NumTurns:     ev.Session.NumTurns,
				Status:       ev.Session.Status,
				Action:       ev.Session.Action,
				StartedAt:    ev.Session.StartedAt,
				EndedAt:      ev.Session.EndedAt,
			}
		}
		if len(ev.Spawns) > 0 {
			spawns := make([]session.WireSpawnInfo, 0, len(ev.Spawns))
			for i := range ev.Spawns {
				spawns = append(spawns, session.WireSpawnInfo{
					ID:            ev.Spawns[i].ID,
					ParentTurnID:  ev.Spawns[i].ParentTurnID,
					ParentSpawnID: ev.Spawns[i].ParentSpawnID,
					ChildTurnID:   ev.Spawns[i].ChildTurnID,
					Profile:       ev.Spawns[i].Profile,
					Position:      ev.Spawns[i].Position,
					Role:          ev.Spawns[i].Role,
					Status:        ev.Spawns[i].Status,
					Question:      ev.Spawns[i].Question,
					Summary:       ev.Spawns[i].Summary,
					Result:        ev.Spawns[i].Result,
				})
			}
			msg.Spawns = spawns
		}
		return wsEnvelope{Type: session.MsgSnapshot, Data: msg}

	case events.AgentStartedMsg:
		return wsEnvelope{Type: session.MsgStarted, Data: session.WireStarted{
			SessionID: ev.SessionID,
			TurnHexID: ev.TurnHexID,
			StepHexID: ev.StepHexID,
			RunHexID:  ev.RunHexID,
		}}

	case events.AgentPromptMsg:
		return wsEnvelope{Type: session.MsgPrompt, Data: session.WirePrompt{
			SessionID:      ev.SessionID,
			TurnHexID:      ev.TurnHexID,
			Prompt:         ev.Prompt,
			IsResume:       ev.IsResume,
			Truncated:      ev.Truncated,
			OriginalLength: ev.OriginalLength,
		}}

	case events.AgentEventMsg:
		eventData, _ := json.Marshal(ev.Event)
		wireEvent := session.WireEvent{
			Event:   json.RawMessage(eventData),
			SpawnID: ev.SpawnID,
			TurnID:  ev.TurnID,
		}
		if len(ev.Raw) > 0 {
			wireEvent.Raw = json.RawMessage(ev.Raw)
		}
		return wsEnvelope{Type: session.MsgEvent, Data: wireEvent}

	case events.AgentRawOutputMsg:
		spawnID := 0
		if ev.SessionID < 0 {
			spawnID = -ev.SessionID
		}
		return wsEnvelope{Type: session.MsgRaw, Data: session.WireRaw{
			Data:      ev.Data,
			SessionID: ev.SessionID,
			SpawnID:   spawnID,
		}}

	case events.AgentFinishedMsg:
		payload := session.WireFinished{
			SessionID:     ev.SessionID,
			TurnHexID:     ev.TurnHexID,
			WaitForSpawns: ev.WaitForSpawns,
		}
		if ev.Result != nil {
			payload.ExitCode = ev.Result.ExitCode
			payload.DurationNS = int64(ev.Result.Duration)
		}
		if ev.Err != nil {
			payload.Error = ev.Err.Error()
		}
		return wsEnvelope{Type: session.MsgFinished, Data: payload}

	case events.SpawnStatusMsg:
		spawns := make([]session.WireSpawnInfo, 0, len(ev.Spawns))
		for i := range ev.Spawns {
			spawns = append(spawns, session.WireSpawnInfo{
				ID:            ev.Spawns[i].ID,
				ParentTurnID:  ev.Spawns[i].ParentTurnID,
				ParentSpawnID: ev.Spawns[i].ParentSpawnID,
				ChildTurnID:   ev.Spawns[i].ChildTurnID,
				Profile:       ev.Spawns[i].Profile,
				Position:      ev.Spawns[i].Position,
				Role:          ev.Spawns[i].Role,
				Status:        ev.Spawns[i].Status,
				Question:      ev.Spawns[i].Question,
				Summary:       ev.Spawns[i].Summary,
				Result:        ev.Spawns[i].Result,
			})
		}
		return wsEnvelope{Type: session.MsgSpawn, Data: session.WireSpawn{Spawns: spawns}}

	case events.LoopStepStartMsg:
		return wsEnvelope{Type: session.MsgLoopStepStart, Data: session.WireLoopStepStart{
			RunID:      ev.RunID,
			RunHexID:   ev.RunHexID,
			StepHexID:  ev.StepHexID,
			Cycle:      ev.Cycle,
			StepIndex:  ev.StepIndex,
			Profile:    ev.Profile,
			Turns:      ev.Turns,
			TotalSteps: ev.TotalSteps,
		}}

	case events.LoopStepEndMsg:
		return wsEnvelope{Type: session.MsgLoopStepEnd, Data: session.WireLoopStepEnd{
			RunID:      ev.RunID,
			RunHexID:   ev.RunHexID,
			StepHexID:  ev.StepHexID,
			Cycle:      ev.Cycle,
			StepIndex:  ev.StepIndex,
			Profile:    ev.Profile,
			TotalSteps: ev.TotalSteps,
		}}

	case events.LoopDoneMsg:
		payload := session.WireLoopDone{
			RunID:    ev.RunID,
			RunHexID: ev.RunHexID,
			Reason:   ev.Reason,
		}
		if ev.Err != nil {
			payload.Error = ev.Err.Error()
		}
		return wsEnvelope{Type: session.MsgLoopDone, Data: payload}

	case events.AgentLoopDoneMsg:
		payload := session.WireDone{}
		if ev.Err != nil {
			payload.Error = ev.Err.Error()
		}
		return wsEnvelope{Type: session.MsgDone, Data: payload}

	default:
		return wsEnvelope{Type: "event", Data: event}
	}
}
