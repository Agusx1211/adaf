package webserver

import (
	"encoding/base64"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/terminal"
	ws, err := websocket.Dial(wsURL, "", ts.URL)
	if err != nil {
		t.Fatalf("websocket.Dial: %v", err)
	}
	defer ws.Close()

	if err := websocket.JSON.Send(ws, terminalWSMessage{Type: "resize", Cols: 120, Rows: 32}); err != nil {
		t.Fatalf("send resize: %v", err)
	}

	input := "echo __adaf_terminal_test__\r\n"
	encodedInput := base64.StdEncoding.EncodeToString([]byte(input))
	if err := websocket.JSON.Send(ws, terminalWSMessage{Type: "input", Data: encodedInput}); err != nil {
		t.Fatalf("send input: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	if err := ws.SetDeadline(deadline); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}

	var combinedOutput strings.Builder
	for {
		var msg terminalWSMessage
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			t.Fatalf("receive message: %v (output=%q)", err, combinedOutput.String())
		}
		if msg.Type != "output" || msg.Data == "" {
			if time.Now().After(deadline) {
				break
			}
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

	t.Fatalf("expected terminal output not found; output=%q", combinedOutput.String())
}
