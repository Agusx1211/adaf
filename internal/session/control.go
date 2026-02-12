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
	conn, err := net.DialTimeout("unix", SocketPath(sessionID), 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to session daemon: %w", err)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 2*1024*1024)

	line, err := EncodeMsg(MsgControl, WireControl{
		Action: "spawn",
		Spawn:  &req,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding control request: %w", err)
	}
	if _, err := conn.Write(line); err != nil {
		return nil, fmt.Errorf("sending control request: %w", err)
	}

	deadline := time.Now().Add(controlRequestTimeout)

	for {
		if !req.Wait {
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
		if resp.Action != "spawn" {
			continue
		}
		return resp, nil
	}
}
