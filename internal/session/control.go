package session

import (
	"bufio"
	"fmt"
	"net"
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
	conn, err := net.DialTimeout("unix", SocketPath(sessionID), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to session daemon: %w", err)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 2*1024*1024)

	line, err := EncodeMsg(MsgControl, req)
	if err != nil {
		return nil, fmt.Errorf("encoding control request: %w", err)
	}
	if _, err := conn.Write(line); err != nil {
		return nil, fmt.Errorf("sending control request: %w", err)
	}

	deadline := time.Now().Add(controlRequestTimeout)

	for {
		if !waitForCompletion {
			_ = conn.SetReadDeadline(deadline)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("reading control response: %w", err)
			}
			return nil, fmt.Errorf("connection closed before control response")
		}

		msg, err := DecodeMsg(scanner.Bytes())
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
