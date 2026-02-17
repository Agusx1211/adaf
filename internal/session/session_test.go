package session

import (
	"context"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/store"
)

func TestAbortSessionStartupMarksMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionID, err := CreateSession(DaemonConfig{
		ProjectDir:  t.TempDir(),
		ProjectName: "proj",
		WorkDir:     t.TempDir(),
		ProfileName: "p1",
		AgentName:   "generic",
		Loop: config.LoopDef{
			Name: "test",
			Steps: []config.LoopStep{
				{Profile: "p1", Turns: 1},
			},
		},
		Profiles: []config.Profile{{Name: "p1", Agent: "generic"}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	AbortSessionStartup(sessionID, "connect failed")

	meta, err := LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if meta.Status != StatusCancelled {
		t.Fatalf("status = %q, want %q", meta.Status, StatusCancelled)
	}
	if !strings.Contains(meta.Error, "connect failed") {
		t.Fatalf("error = %q, want to contain %q", meta.Error, "connect failed")
	}
	if meta.EndedAt.IsZero() {
		t.Fatal("EndedAt is zero, want non-zero")
	}
}

func TestCreateSessionPopulatesProjectID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectDir := t.TempDir()
	s, err := store.New(projectDir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	if err := s.Init(store.ProjectConfig{Name: "proj", RepoPath: projectDir}); err != nil {
		t.Fatalf("store.Init: %v", err)
	}

	sessionID, err := CreateSession(DaemonConfig{
		ProjectDir:  projectDir,
		ProjectName: "proj",
		WorkDir:     t.TempDir(),
		ProfileName: "p1",
		AgentName:   "generic",
		Loop: config.LoopDef{
			Name: "test",
			Steps: []config.LoopStep{
				{Profile: "p1", Turns: 1},
			},
		},
		Profiles: []config.Profile{{Name: "p1", Agent: "generic"}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	meta, err := LoadMeta(sessionID)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}

	expectedID := ProjectIDFromDir(projectDir)
	if meta.ProjectID != expectedID {
		t.Fatalf("ProjectID = %q, want %q", meta.ProjectID, expectedID)
	}

	// Format check: <readable>-<uuid-v4>
	pattern := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(meta.ProjectID) {
		t.Fatalf("ProjectID %q does not match expected format", meta.ProjectID)
	}

	// Determinism check (marker-based).
	if ProjectIDFromDir(projectDir) != expectedID {
		t.Fatal("ProjectID is not deterministic")
	}
}

func TestSendCancelControlUsesWebSocket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	const sessionID = 78
	if err := os.MkdirAll(SessionDir(sessionID), 0755); err != nil {
		t.Fatalf("MkdirAll(SessionDir): %v", err)
	}

	sockPath := SocketPath(sessionID)
	_ = os.Remove(sockPath)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("Listen(unix): %v", err)
	}
	defer listener.Close()

	cancelMsgCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer ws.CloseNow()

		metaLine, _ := EncodeMsg(MsgMeta, WireMeta{SessionID: sessionID})
		if len(metaLine) > 0 && metaLine[len(metaLine)-1] == '\n' {
			metaLine = metaLine[:len(metaLine)-1]
		}
		if err := ws.Write(r.Context(), websocket.MessageText, metaLine); err != nil {
			return
		}

		_, data, err := ws.Read(r.Context())
		if err != nil {
			return
		}
		select {
		case cancelMsgCh <- string(data):
		default:
		}
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

	if err := sendCancelControl(sessionID); err != nil {
		t.Fatalf("sendCancelControl: %v", err)
	}

	select {
	case got := <-cancelMsgCh:
		if got != CtrlCancel {
			t.Fatalf("cancel message = %q, want %q", got, CtrlCancel)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancel control message")
	}
}

func TestCreateSessionConcurrentUniqueIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	projectDir := t.TempDir()

	const n = 24
	ids := make(chan int, n)
	errs := make(chan error, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id, err := CreateSession(DaemonConfig{
				ProjectDir:  projectDir,
				ProjectName: "proj",
				WorkDir:     projectDir,
				ProfileName: "p1",
				AgentName:   "generic",
				Loop: config.LoopDef{
					Name: "test",
					Steps: []config.LoopStep{
						{Profile: "p1", Turns: 1},
					},
				},
				Profiles: []config.Profile{{Name: "p1", Agent: "generic"}},
			})
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Fatalf("CreateSession concurrent error: %v", err)
	}

	seen := make(map[int]struct{}, n)
	for id := range ids {
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate session id detected: %d", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("created sessions = %d, want %d", len(seen), n)
	}
}

func TestIsAgentContext(t *testing.T) {
	t.Run("false when no marker env vars are set", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "")
		t.Setenv("ADAF_AGENT", "")
		if IsAgentContext() {
			t.Fatal("IsAgentContext() = true, want false")
		}
	})

	t.Run("true when ADAF_TURN_ID is set", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "123")
		t.Setenv("ADAF_SESSION_ID", "")
		t.Setenv("ADAF_AGENT", "")
		if !IsAgentContext() {
			t.Fatal("IsAgentContext() = false, want true")
		}
	})

	t.Run("true when ADAF_SESSION_ID is set", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "55")
		t.Setenv("ADAF_AGENT", "")
		if !IsAgentContext() {
			t.Fatal("IsAgentContext() = false, want true")
		}
	})

	t.Run("true when ADAF_AGENT is 1", func(t *testing.T) {
		t.Setenv("ADAF_TURN_ID", "")
		t.Setenv("ADAF_SESSION_ID", "")
		t.Setenv("ADAF_AGENT", "1")
		if !IsAgentContext() {
			t.Fatal("IsAgentContext() = false, want true")
		}
	})
}

func TestIsActiveStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "starting", status: StatusStarting, want: true},
		{name: "running", status: StatusRunning, want: true},
		{name: "done", status: StatusDone, want: false},
		{name: "cancelled", status: StatusCancelled, want: false},
		{name: "error", status: StatusError, want: false},
		{name: "dead", status: StatusDead, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsActiveStatus(tt.status)
			if got != tt.want {
				t.Fatalf("IsActiveStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
