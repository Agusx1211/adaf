package webserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"
)

const (
	terminalDefaultRows   = 24
	terminalDefaultCols   = 80
	terminalReadBufferLen = 4096
)

type terminalWSMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
}

func (srv *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer ws.CloseNow()

	ctx := r.Context()

	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell)
	cmd.Dir = srv.terminalWorkDir()
	attrs := &syscall.SysProcAttr{Setpgid: true}
	cmd.SysProcAttr = attrs

	ptmx, err := pty.StartWithAttrs(cmd, nil, attrs)
	if err != nil {
		data, _ := json.Marshal(terminalWSMessage{Type: "exit", Code: 1})
		_ = ws.Write(ctx, websocket.MessageText, data)
		ws.Close(websocket.StatusInternalError, "pty start failed")
		return
	}

	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: terminalDefaultRows, Cols: terminalDefaultCols})

	var (
		writeMu     sync.Mutex
		cleanupOnce sync.Once
	)

	send := func(msg terminalWSMessage) error {
		data, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		writeCtx, writeCancel := context.WithTimeout(ctx, 15*time.Second)
		defer writeCancel()
		return ws.Write(writeCtx, websocket.MessageText, data)
	}

	cleanup := func() {
		cleanupOnce.Do(func() {
			_ = ptmx.Close()
			if cmd.Process != nil && cmd.Process.Pid > 0 {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		})
	}
	defer cleanup()

	go func() {
		buf := make([]byte, terminalReadBufferLen)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				out := terminalWSMessage{
					Type: "output",
					Data: base64.StdEncoding.EncodeToString(buf[:n]),
				}
				if err := send(out); err != nil {
					cleanup()
					ws.CloseNow()
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return
				}
				return
			}
		}
	}()

	go func() {
		err := cmd.Wait()
		_ = send(terminalWSMessage{Type: "exit", Code: exitCode(err)})
		cleanup()
		ws.Close(websocket.StatusNormalClosure, "process exited")
	}()

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			cleanup()
			return
		}

		var msg terminalWSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "input":
			if msg.Data == "" {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(msg.Data)
			if err != nil || len(decoded) == 0 {
				continue
			}
			if _, err := ptmx.Write(decoded); err != nil {
				cleanup()
				return
			}

		case "resize":
			if msg.Cols <= 0 || msg.Rows <= 0 {
				continue
			}
			_ = pty.Setsize(ptmx, &pty.Winsize{
				Rows: clampToUint16(msg.Rows),
				Cols: clampToUint16(msg.Cols),
			})
		}
	}
}

func (srv *Server) terminalWorkDir() string {
	if srv != nil && srv.store != nil {
		if root := strings.TrimSpace(srv.store.Root()); root != "" {
			return root
		}
	}

	cwd, err := os.Getwd()
	if err == nil && strings.TrimSpace(cwd) != "" {
		return cwd
	}

	return "."
}

func clampToUint16(value int) uint16 {
	if value < 1 {
		return 1
	}
	if value > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(value)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return 1
}
