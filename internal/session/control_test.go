package session

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRequestSpawn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const sessionID = 77
	if err := os.MkdirAll(SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll(SessionDir): %v", err)
	}

	// Start an HTTP server on the Unix socket path for the session.
	sockPath := SocketPath(sessionID)
	os.Remove(sockPath)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Listen(unix): %v", err)
	}
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer ws.CloseNow()

		ctx := r.Context()

		// Send meta.
		metaLine, _ := EncodeMsg(MsgMeta, WireMeta{SessionID: sessionID})
		if len(metaLine) > 0 && metaLine[len(metaLine)-1] == '\n' {
			metaLine = metaLine[:len(metaLine)-1]
		}
		if err := ws.Write(ctx, websocket.MessageText, metaLine); err != nil {
			return
		}

		// Send live marker (snapshot is empty).
		snapLine, _ := EncodeMsg(MsgSnapshot, WireSnapshot{})
		if len(snapLine) > 0 && snapLine[len(snapLine)-1] == '\n' {
			snapLine = snapLine[:len(snapLine)-1]
		}
		_ = ws.Write(ctx, websocket.MessageText, snapLine)

		liveLine, _ := EncodeMsg(MsgLive, nil)
		if len(liveLine) > 0 && liveLine[len(liveLine)-1] == '\n' {
			liveLine = liveLine[:len(liveLine)-1]
		}
		_ = ws.Write(ctx, websocket.MessageText, liveLine)

		// Read control request.
		_, _, err = ws.Read(ctx)
		if err != nil {
			return
		}

		// Send response.
		respLine, _ := EncodeMsg(MsgControlResult, WireControlResult{
			Action:  "spawn",
			OK:      true,
			SpawnID: 9,
		})
		if len(respLine) > 0 && respLine[len(respLine)-1] == '\n' {
			respLine = respLine[:len(respLine)-1]
		}
		_ = ws.Write(ctx, websocket.MessageText, respLine)

		// Keep connection alive briefly.
		time.Sleep(100 * time.Millisecond)
		ws.Close(websocket.StatusNormalClosure, "done")
	})

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		_ = httpServer.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	})

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
}
