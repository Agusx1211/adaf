package looprun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agusx1211/adaf/internal/events"
	"github.com/agusx1211/adaf/internal/store"
)

func TestEmitSpawnOutput_PartialJSONLineIsRetried(t *testing.T) {
	s := newLooprunTestStore(t)
	childTurnID := 4242
	spawnID := 99
	records := []store.SpawnRecord{
		{ID: spawnID, ChildTurnID: childTurnID},
	}
	offsets := make(map[int]int64)
	eventCh := make(chan any, 4)

	eventsPath := filepath.Join(s.Root(), "local", "records", fmt.Sprintf("%d", childTurnID), "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(eventsPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ev := store.RecordingEvent{
		Timestamp: time.Unix(1, 0).UTC(),
		Type:      "stdout",
		Data:      "hello from child",
	}
	line, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	partial := line[:len(line)/2]
	if err := os.WriteFile(eventsPath, partial, 0o644); err != nil {
		t.Fatalf("WriteFile(partial): %v", err)
	}

	emitSpawnOutput(records, s, offsets, eventCh)
	if got := offsets[spawnID]; got != 0 {
		t.Fatalf("offset after partial read = %d, want 0", got)
	}
	select {
	case msg := <-eventCh:
		t.Fatalf("unexpected event emitted for partial JSON line: %#v", msg)
	default:
	}

	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(append): %v", err)
	}
	if _, err := f.Write(append(line[len(partial):], '\n')); err != nil {
		_ = f.Close()
		t.Fatalf("append remainder: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	emitSpawnOutput(records, s, offsets, eventCh)
	select {
	case raw := <-eventCh:
		msg, ok := raw.(events.AgentRawOutputMsg)
		if !ok {
			t.Fatalf("event type = %T, want events.AgentRawOutputMsg", raw)
		}
		if msg.SessionID != -spawnID {
			t.Fatalf("SessionID = %d, want %d", msg.SessionID, -spawnID)
		}
		if msg.Data != ev.Data {
			t.Fatalf("Data = %q, want %q", msg.Data, ev.Data)
		}
	default:
		t.Fatal("expected output event after completing partial JSON line")
	}
}
