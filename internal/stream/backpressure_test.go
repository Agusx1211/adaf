package stream

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestParsersDoNotBlockOnSlowConsumers(t *testing.T) {
	tests := []struct {
		name  string
		parse func(context.Context, io.Reader) <-chan RawEvent
		line  string
	}{
		{
			name:  "claude",
			parse: Parse,
			line:  `{"type":"system","subtype":"init","session_id":"s1"}`,
		},
		{
			name:  "codex",
			parse: ParseCodex,
			line:  `{"type":"thread.started","thread_id":"thread-1"}`,
		},
		{
			name:  "gemini",
			parse: ParseGemini,
			line:  `{"type":"init","session_id":"gem-1","model":"gemini-2.5-pro"}`,
		},
		{
			name:  "opencode",
			parse: ParseOpencode,
			line:  `{"type":"step_start","sessionID":"open-1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertParserWriteCompletesWithoutConsumer(t, tt.parse, tt.line)
		})
	}
}

func assertParserWriteCompletesWithoutConsumer(t *testing.T, parse func(context.Context, io.Reader) <-chan RawEvent, line string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pr, pw := io.Pipe()
	ch := parse(ctx, pr)

	writeDone := make(chan error, 1)
	go func() {
		for i := 0; i < 5000; i++ {
			if _, err := io.WriteString(pw, line+"\n"); err != nil {
				writeDone <- err
				return
			}
		}
		writeDone <- pw.Close()
	}()

	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("writer returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writer blocked; parser likely back-pressured on output channel")
	}

	cancel()
	drainDone := make(chan struct{})
	go func() {
		for range ch {
		}
		close(drainDone)
	}()
	select {
	case <-drainDone:
	case <-time.After(2 * time.Second):
		t.Fatal("parser channel did not close after cancellation")
	}
}
