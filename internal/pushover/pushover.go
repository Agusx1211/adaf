// Package pushover implements the Pushover notification API client.
package pushover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/agusx1211/adaf/internal/config"
)

const (
	apiURL = "https://api.pushover.net/1/messages.json"

	// MaxTitleLen is the maximum length for a Pushover notification title.
	MaxTitleLen = 250

	// MaxMessageLen is the maximum length for a Pushover notification message.
	MaxMessageLen = 1024
)

// Priority levels for Pushover notifications.
const (
	PriorityLowest = -2
	PriorityLow    = -1
	PriorityNormal = 0
	PriorityHigh   = 1
)

// Message represents a Pushover notification to send.
type Message struct {
	Title    string
	Body     string
	Priority int
}

// Response is the JSON response from the Pushover API.
type Response struct {
	Status  int      `json:"status"`
	Request string   `json:"request"`
	Errors  []string `json:"errors,omitempty"`
}

// Send sends a Pushover notification using the credentials from the global config.
func Send(cfg *config.PushoverConfig, msg Message) error {
	if cfg.UserKey == "" || cfg.AppToken == "" {
		return fmt.Errorf("pushover not configured: run 'adaf config pushover setup' to set credentials")
	}

	title := msg.Title
	if len(title) > MaxTitleLen {
		title = title[:MaxTitleLen]
	}

	body := msg.Body
	if len(body) > MaxMessageLen {
		body = body[:MaxMessageLen]
	}

	form := url.Values{
		"token":    {cfg.AppToken},
		"user":     {cfg.UserKey},
		"title":    {title},
		"message":  {body},
		"priority": {fmt.Sprintf("%d", msg.Priority)},
	}

	resp, err := http.Post(apiURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("sending pushover notification: %w", err)
	}
	defer resp.Body.Close()

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding pushover response: %w", err)
	}

	if result.Status != 1 {
		return fmt.Errorf("pushover API error: %s", strings.Join(result.Errors, "; "))
	}

	return nil
}

// Configured returns true if Pushover credentials are set.
func Configured(cfg *config.PushoverConfig) bool {
	return cfg.UserKey != "" && cfg.AppToken != ""
}
