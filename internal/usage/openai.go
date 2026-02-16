package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type OpenAIProvider struct {
	apiKey            string
	baseURL           string
	warnThreshold     float64
	criticalThreshold float64
}

func NewOpenAIProvider(opts ...OpenAIOption) *OpenAIProvider {
	p := &OpenAIProvider{
		baseURL:           "https://api.openai.com/v1",
		warnThreshold:     70.0,
		criticalThreshold: 90.0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

type OpenAIOption func(*OpenAIProvider)

func WithOpenAIAPIKey(key string) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.apiKey = key
	}
}

func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.baseURL = url
	}
}

func WithOpenAIThresholds(warn, critical float64) OpenAIOption {
	return func(p *OpenAIProvider) {
		p.warnThreshold = warn
		p.criticalThreshold = critical
	}
}

func (p *OpenAIProvider) Name() ProviderKind {
	return ProviderOpenAI
}

func (p *OpenAIProvider) HasCredentials() bool {
	key, err := p.getAPIKey()
	return err == nil && key != ""
}

func (p *OpenAIProvider) getAPIKey() (string, error) {
	if p.apiKey != "" {
		return p.apiKey, nil
	}

	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		return key, nil
	}

	return getOpenAIKeyFromCodexAuth()
}

func getOpenAIKeyFromCodexAuth() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	path := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	var creds struct {
		AuthMode string `json:"auth_mode"`
		Tokens   struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}

	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if creds.AuthMode == "chatgpt" {
		return "", fmt.Errorf("codex is using ChatGPT auth (no API scopes)")
	}

	if creds.Tokens.AccessToken == "" {
		return "", fmt.Errorf("no access_token in %s", path)
	}

	return creds.Tokens.AccessToken, nil
}

func (p *OpenAIProvider) FetchUsage(ctx context.Context) (UsageSnapshot, error) {
	key, err := p.getAPIKey()
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderOpenAI, Err: err}
	}

	reqBody := map[string]interface{}{
		"model":      "gpt-4o-mini",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	url := strings.TrimSuffix(p.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderOpenAI, Err: err}
	}

	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderOpenAI, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderOpenAI,
			Err:      fmt.Errorf("API key is invalid"),
		}
	}

	io.Copy(io.Discard, resp.Body)

	var limits []UsageLimit

	if limit, remaining := headerFloat(resp.Header, "x-ratelimit-limit-requests"), headerFloat(resp.Header, "x-ratelimit-remaining-requests"); limit > 0 {
		used := limit - remaining
		limits = append(limits, UsageLimit{
			Name:           "Requests",
			UtilizationPct: (used / limit) * 100,
		})
	}

	if limit, remaining := headerFloat(resp.Header, "x-ratelimit-limit-tokens"), headerFloat(resp.Header, "x-ratelimit-remaining-tokens"); limit > 0 {
		used := limit - remaining
		limits = append(limits, UsageLimit{
			Name:           "Tokens",
			UtilizationPct: (used / limit) * 100,
		})
	}

	if len(limits) == 0 {
		limits = append(limits, UsageLimit{
			Name:           "Rate limit",
			UtilizationPct: 0,
		})
	}

	return NewSnapshot(ProviderOpenAI, limits, p.warnThreshold, p.criticalThreshold), nil
}
