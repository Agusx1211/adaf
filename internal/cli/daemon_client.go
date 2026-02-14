package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/session"
)

type DaemonClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

type daemonErrorResponse struct {
	Error string `json:"error"`
}

// TryConnect checks for a running daemon and returns a client, or nil if none.
func TryConnect() *DaemonClient {
	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil || !running || state.PID == 0 {
		return nil
	}
	if !isPIDAlive(state.PID) {
		return nil
	}

	baseURL := strings.TrimSpace(state.URL)
	if baseURL == "" && state.Port > 0 {
		scheme := strings.TrimSpace(state.Scheme)
		if scheme == "" {
			scheme = "http"
		}
		host := strings.TrimSpace(state.Host)
		if host == "" {
			host = "127.0.0.1"
		}
		baseURL = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(state.Port)))
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}

	return &DaemonClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		Token: strings.TrimSpace(os.Getenv("ADAF_WEB_AUTH_TOKEN")),
	}
}

func (c *DaemonClient) CreateIssue(projectID string, req map[string]interface{}) error {
	path := fmt.Sprintf("/api/projects/%s/issues", url.PathEscape(strings.TrimSpace(projectID)))
	return c.doJSON(http.MethodPost, path, req, http.StatusCreated)
}

func (c *DaemonClient) UpdateIssue(projectID, issueID string, req map[string]interface{}) error {
	path := fmt.Sprintf(
		"/api/projects/%s/issues/%s",
		url.PathEscape(strings.TrimSpace(projectID)),
		url.PathEscape(strings.TrimSpace(issueID)),
	)
	return c.doJSON(http.MethodPut, path, req, http.StatusOK)
}

func (c *DaemonClient) CreatePlan(projectID string, req map[string]interface{}) error {
	path := fmt.Sprintf("/api/projects/%s/plans", url.PathEscape(strings.TrimSpace(projectID)))
	return c.doJSON(http.MethodPost, path, req, http.StatusCreated)
}

func (c *DaemonClient) doJSON(method, path string, req map[string]interface{}, okStatuses ...int) error {
	if c == nil {
		return fmt.Errorf("daemon client is nil")
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 5 * time.Second}
	}

	var body []byte
	if req != nil {
		data, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("encoding request: %w", err)
		}
		body = data
	} else {
		body = []byte("{}")
	}

	reqURL := strings.TrimRight(c.BaseURL, "/") + path
	httpReq, err := http.NewRequest(method, reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(c.Token); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("daemon request failed: %w", err)
	}
	defer resp.Body.Close()

	if len(okStatuses) == 0 {
		okStatuses = []int{http.StatusOK}
	}
	for _, status := range okStatuses {
		if resp.StatusCode == status {
			return nil
		}
	}

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	if len(respBody) > 0 {
		var daemonErr daemonErrorResponse
		if err := json.Unmarshal(respBody, &daemonErr); err == nil && strings.TrimSpace(daemonErr.Error) != "" {
			return fmt.Errorf("daemon request failed (%d): %s", resp.StatusCode, daemonErr.Error)
		}
	}
	return fmt.Errorf("daemon request failed with status %d", resp.StatusCode)
}

func projectIDFromPath(projectDir string) string {
	cleanedPath := normalizeProjectDir(projectDir)

	// Check registry first for existing ID
	registry, err := loadWebProjectRegistry(webProjectsRegistryPath())
	if err == nil && registry != nil {
		for _, project := range registry.Projects {
			if normalizeProjectDir(project.Path) == cleanedPath && project.ID != "" {
				return project.ID
			}
		}
	}

	// Derive using the canonical function
	return session.ProjectIDFromDir(cleanedPath)
}

func normalizeProjectDir(projectDir string) string {
	path := strings.TrimSpace(projectDir)
	if path == "" {
		return "."
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	return filepath.Clean(path)
}
