package webserver

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestReportMissingUISampleEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)

	body := `{
		"source":"session_ws_event",
		"reason":"unknown_assistant_block",
		"scope":"session-12",
		"session_id":12,
		"event_type":"tool_invocation_v2",
		"agent":"codex",
		"model":"gpt-5.2-codex",
		"fallback_text":"[tool_invocation_v2] {...}",
		"payload":{"type":"tool_invocation_v2","foo":"bar"}
	}`

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/projects/test-project/ui/missing-samples", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp struct {
		OK   bool   `json:"ok"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.OK {
		t.Fatal("ok = false, want true")
	}
	if strings.TrimSpace(resp.Path) == "" {
		t.Fatal("path is empty")
	}
	if !strings.Contains(resp.Path, string(os.PathSeparator)+"missing_UIs"+string(os.PathSeparator)) {
		t.Fatalf("path = %q, want to contain missing_UIs directory", resp.Path)
	}

	data, err := os.ReadFile(resp.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", resp.Path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("missing persisted JSONL lines")
	}

	var got missingUISampleRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
		t.Fatalf("unmarshal persisted record: %v", err)
	}

	if got.Source != "session_ws_event" {
		t.Fatalf("source = %q, want %q", got.Source, "session_ws_event")
	}
	if got.Reason != "unknown_assistant_block" {
		t.Fatalf("reason = %q, want %q", got.Reason, "unknown_assistant_block")
	}
	if got.ProjectID != "test-project" {
		t.Fatalf("project_id = %q, want %q", got.ProjectID, "test-project")
	}
	if got.Provider != "openai" {
		t.Fatalf("provider = %q, want %q", got.Provider, "openai")
	}

	var payload map[string]any
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["type"] != "tool_invocation_v2" {
		t.Fatalf("payload.type = %#v, want %q", payload["type"], "tool_invocation_v2")
	}
}

func TestReportMissingUISampleEndpoint_ValidatesRequiredFields(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := performJSONRequest(t, srv, http.MethodPost, "/api/ui/missing-samples", `{"source":"session_ws_event"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
