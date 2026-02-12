package session

import (
	"bufio"
	"net"
	"os"
	"testing"
)

func TestRequestSpawn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const sessionID = 77
	if err := os.MkdirAll(SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll(SessionDir): %v", err)
	}

	listener, err := net.Listen("unix", SocketPath(sessionID))
	if err != nil {
		t.Fatalf("Listen(unix): %v", err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()

		metaLine, _ := EncodeMsg(MsgMeta, WireMeta{SessionID: sessionID})
		if _, err := conn.Write(metaLine); err != nil {
			serverErr <- err
			return
		}
		liveLine, _ := EncodeMsg(MsgLive, nil)
		if _, err := conn.Write(liveLine); err != nil {
			serverErr <- err
			return
		}

		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 1024), 1024*1024)
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				serverErr <- err
				return
			}
			serverErr <- nil
			return
		}

		respLine, _ := EncodeMsg(MsgControlResult, WireControlResult{
			Action:  "spawn",
			OK:      true,
			SpawnID: 9,
		})
		_, err = conn.Write(respLine)
		serverErr <- err
	}()

	resp, err := RequestSpawn(sessionID, WireControlSpawn{
		ParentTurnID:  1,
		ParentProfile: "manager",
		ChildProfile:  "devstral2",
		Task:          "review files",
	})
	if err != nil {
		t.Fatalf("RequestSpawn: %v", err)
	}
	if resp == nil {
		t.Fatal("RequestSpawn returned nil response")
	}
	if !resp.OK {
		t.Fatalf("response ok=false: %+v", resp)
	}
	if resp.SpawnID != 9 {
		t.Fatalf("spawn_id = %d, want 9", resp.SpawnID)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}
