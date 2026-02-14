package webserver

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestTerminalWebSocketInputOutput(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("/bin/sh not available: %v", err)
	}
	// Use a predictable shell for stable PTY behavior in test runs.
	t.Setenv("SHELL", "/bin/sh")

	srv, _ := newTestServer(t)
	ts := httptest.NewServer(srv.httpServer.Handler)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/terminal"
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer ws.Close(websocket.StatusNormalClosure, "test finished")

	if err := wsjson.Write(ctx, ws, terminalWSMessage{Type: "resize", Cols: 120, Rows: 32}); err != nil {
		t.Fatalf("send resize: %v", err)
	}

	input := "echo __adaf_terminal_test__\r\n"
	encodedInput := base64.StdEncoding.EncodeToString([]byte(input))
	if err := wsjson.Write(ctx, ws, terminalWSMessage{Type: "input", Data: encodedInput}); err != nil {
		t.Fatalf("send input: %v", err)
	}

	var combinedOutput strings.Builder
	for {
		var msg terminalWSMessage
		if err := wsjson.Read(ctx, ws, &msg); err != nil {
			t.Fatalf("receive message: %v (output=%q)", err, combinedOutput.String())
		}
		if msg.Type != "output" || msg.Data == "" {
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(msg.Data)
		if err != nil {
			t.Fatalf("decode output data: %v", err)
		}
		combinedOutput.Write(decoded)
		if strings.Contains(combinedOutput.String(), "__adaf_terminal_test__") {
			return
		}
	}
}
