package webserver

import (
	"encoding/base64"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/net/websocket"
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
	h := websocket.Handler(func(ws *websocket.Conn) {
		defer ws.Close()

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
			_ = websocket.JSON.Send(ws, terminalWSMessage{Type: "exit", Code: 1})
			return
		}

		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: terminalDefaultRows, Cols: terminalDefaultCols})

		var (
			writeMu     sync.Mutex
			cleanupOnce sync.Once
		)

		send := func(msg terminalWSMessage) error {
			writeMu.Lock()
			defer writeMu.Unlock()
			return websocket.JSON.Send(ws, msg)
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
						_ = ws.Close()
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
			_ = ws.Close()
		}()

		for {
			var msg terminalWSMessage
			if err := websocket.JSON.Receive(ws, &msg); err != nil {
				cleanup()
				return
			}

			switch msg.Type {
			case "input":
				if msg.Data == "" {
					continue
				}
				data, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil || len(data) == 0 {
					continue
				}
				if _, err := ptmx.Write(data); err != nil {
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
	})

	h.ServeHTTP(w, r)
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
