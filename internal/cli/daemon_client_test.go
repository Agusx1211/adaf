package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectIDFromPathDeterministicForUnregisteredPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	projectDir := filepath.Join(t.TempDir(), "My Demo Project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("creating project dir: %v", err)
	}

	got1 := projectIDFromPath(projectDir)
	got2 := projectIDFromPath(projectDir)
	if got1 != got2 {
		t.Fatalf("projectIDFromPath() not deterministic: %q vs %q", got1, got2)
	}
	if !strings.HasPrefix(got1, "my-demo-project-") {
		t.Fatalf("projectIDFromPath() prefix = %q, want prefix %q", got1, "my-demo-project-")
	}
}

func TestDaemonClientCreateIssueSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/api/projects/project-id/issues" {
			t.Fatalf("path = %s, want %s", r.URL.Path, "/api/projects/project-id/issues")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization header = %q, want %q", got, "Bearer test-token")
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req["title"] != "Sample issue" {
			t.Fatalf("title = %v, want %v", req["title"], "Sample issue")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := &DaemonClient{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Token:      "test-token",
	}
	err := client.CreateIssue("project-id", map[string]interface{}{
		"title": "Sample issue",
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
}

func TestDaemonClientCreateIssueReturnsDaemonError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid issue status"}`))
	}))
	defer srv.Close()

	client := &DaemonClient{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}
	err := client.CreateIssue("project-id", map[string]interface{}{
		"title": "Sample issue",
	})
	if err == nil {
		t.Fatal("CreateIssue() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "invalid issue status") {
		t.Fatalf("CreateIssue() error = %q, want daemon error message", err.Error())
	}
}
