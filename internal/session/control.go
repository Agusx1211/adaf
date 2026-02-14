package session

import (
	"context"
	"fmt"
	"time"
)

const controlRequestTimeout = 30 * time.Second

// RequestSpawn asks a running session daemon to execute a spawn request.
// It returns the daemon's control response.
func RequestSpawn(sessionID int, req WireControlSpawn) (*WireControlResult, error) {
	return requestControl(sessionID, WireControl{
		Action: "spawn",
		Spawn:  &req,
	}, "spawn", req.Wait)
}

// RequestWait asks a running session daemon to create a wait-for-spawns signal
// for a specific turn.
func RequestWait(sessionID int, turnID int) (*WireControlResult, error) {
	return requestControl(sessionID, WireControl{
		Action: "wait",
		Wait: &WireControlWait{
			TurnID: turnID,
		},
	}, "wait", false)
}

// RequestInterruptSpawn asks a running session daemon to interrupt a spawn's
// current turn with a message.
func RequestInterruptSpawn(sessionID int, spawnID int, message string) (*WireControlResult, error) {
	return requestControl(sessionID, WireControl{
		Action: "interrupt_spawn",
		Interrupt: &WireControlInterrupt{
			SpawnID: spawnID,
			Message: message,
		},
	}, "interrupt_spawn", false)
}

func requestControl(sessionID int, req WireControl, expectAction string, waitForCompletion bool) (*WireControlResult, error) {
	client, err := Connect(SocketPath(sessionID))
	if err != nil {
		return nil, fmt.Errorf("connecting to session daemon: %w", err)
	}
	defer client.Close()

	line, err := EncodeMsg(MsgControl, req)
	if err != nil {
		return nil, fmt.Errorf("encoding control request: %w", err)
	}
	// Strip trailing newline for WebSocket frame.
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}

	timeout := controlRequestTimeout
	if waitForCompletion {
		timeout = 10 * time.Minute // spawns can take a long time
	}
	writeCtx, writeCancel := context.WithTimeout(client.ctx, 5*time.Second)
	defer writeCancel()
	if err := client.ws.Write(writeCtx, 1 /* MessageText */, line); err != nil {
		return nil, fmt.Errorf("sending control request: %w", err)
	}

	// Read messages until we get the control result we're looking for.
	readCtx, readCancel := context.WithTimeout(client.ctx, timeout)
	defer readCancel()
	for {
		_, data, err := client.ws.Read(readCtx)
		if err != nil {
			return nil, fmt.Errorf("reading control response: %w", err)
		}

		msg, err := DecodeMsg(data)
		if err != nil {
			continue
		}
		if msg.Type != MsgControlResult {
			continue
		}

		resp, err := DecodeData[WireControlResult](msg)
		if err != nil {
			return nil, fmt.Errorf("decoding control response: %w", err)
		}
		if resp.Action != expectAction {
			continue
		}
		return resp, nil
	}
}
