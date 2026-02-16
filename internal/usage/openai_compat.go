package usage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OpenAICompatProvider fetches rate-limit data from any OpenAI-compatible API
// by issuing a lightweight GET /models request and reading rate-limit headers.
// No tokens are consumed.
type OpenAICompatProvider struct {
	provider          ProviderKind
	baseURL           string
	apiKeyEnvVars     []string
	apiKeyFiles       []apiKeyFile
	warnThreshold     float64
	criticalThreshold float64
}

type apiKeyFile struct {
	path    string // relative to home dir
	envKey  string // key name inside a .env-style file
	literal bool   // if true, treat entire file content as the key
}

type OpenAICompatOption func(*OpenAICompatProvider)

func WithCompatThresholds(warn, critical float64) OpenAICompatOption {
	return func(p *OpenAICompatProvider) {
		p.warnThreshold = warn
		p.criticalThreshold = critical
	}
}

func NewMistralProvider(opts ...OpenAICompatOption) *OpenAICompatProvider {
	p := &OpenAICompatProvider{
		provider:          ProviderMistral,
		baseURL:           "https://api.mistral.ai/v1",
		apiKeyEnvVars:     []string{"MISTRAL_API_KEY"},
		apiKeyFiles:       []apiKeyFile{{path: ".vibe/.env", envKey: "MISTRAL_API_KEY"}},
		warnThreshold:     70.0,
		criticalThreshold: 90.0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func NewZAIProvider(opts ...OpenAICompatOption) *OpenAICompatProvider {
	p := &OpenAICompatProvider{
		provider:          ProviderZAI,
		baseURL:           "https://api.z.ai/api/coding/paas/v4",
		apiKeyEnvVars:     []string{"Z_AI_API_KEY"},
		warnThreshold:     70.0,
		criticalThreshold: 90.0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *OpenAICompatProvider) Name() ProviderKind {
	return p.provider
}

func (p *OpenAICompatProvider) HasCredentials() bool {
	key, err := p.getAPIKey()
	return err == nil && key != ""
}

func (p *OpenAICompatProvider) getAPIKey() (string, error) {
	for _, envVar := range p.apiKeyEnvVars {
		if key := os.Getenv(envVar); key != "" {
			return key, nil
		}
	}

	for _, f := range p.apiKeyFiles {
		if key, err := readKeyFromFile(f); err == nil && key != "" {
			return key, nil
		}
	}

	return "", fmt.Errorf("no API key found for %s", p.provider.DisplayName())
}

func readKeyFromFile(f apiKeyFile) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(home, f.path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if f.literal {
		return strings.TrimSpace(string(data)), nil
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, f.envKey+"=") {
			value := strings.TrimPrefix(line, f.envKey+"=")
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if value != "" {
				return value, nil
			}
		}
	}

	return "", fmt.Errorf("key %s not found in %s", f.envKey, path)
}

func (p *OpenAICompatProvider) FetchUsage(ctx context.Context) (UsageSnapshot, error) {
	key, err := p.getAPIKey()
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: p.provider, Err: err}
	}

	url := strings.TrimSuffix(p.baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: p.provider, Err: err}
	}

	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: p.provider, Err: err}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return UsageSnapshot{}, &ProviderError{
			Provider: p.provider,
			Err:      fmt.Errorf("API key is invalid (HTTP %d)", resp.StatusCode),
		}
	}

	var limits []UsageLimit

	// Try standard OpenAI-style rate limit headers.
	limits = appendHeaderLimit(limits, resp.Header,
		"x-ratelimit-limit-requests", "x-ratelimit-remaining-requests",
		"x-ratelimit-reset-requests", "Requests")
	limits = appendHeaderLimit(limits, resp.Header,
		"x-ratelimit-limit-tokens", "x-ratelimit-remaining-tokens",
		"x-ratelimit-reset-tokens", "Tokens")

	// Mistral uses per-minute and per-month headers.
	limits = appendHeaderLimit(limits, resp.Header,
		"x-ratelimit-limit-req-minute", "x-ratelimit-remaining-req-minute",
		"", "Req/min")
	limits = appendHeaderLimit(limits, resp.Header,
		"x-ratelimit-limit-tokens-minute", "x-ratelimit-remaining-tokens-minute",
		"", "Tokens/min")
	limits = appendHeaderLimit(limits, resp.Header,
		"x-ratelimit-limit-tokens-month", "x-ratelimit-remaining-tokens-month",
		"", "Monthly tokens")

	if len(limits) == 0 {
		return NewSnapshot(p.provider, nil, p.warnThreshold, p.criticalThreshold), nil
	}

	return NewSnapshot(p.provider, limits, p.warnThreshold, p.criticalThreshold), nil
}

func appendHeaderLimit(limits []UsageLimit, h http.Header, limitKey, remainingKey, resetKey, name string) []UsageLimit {
	limit := headerFloat(h, limitKey)
	remaining := headerFloat(h, remainingKey)
	if limit <= 0 {
		return limits
	}

	used := limit - remaining
	pct := (used / limit) * 100

	var resetsAt *time.Time
	if resetKey != "" {
		if v := h.Get(resetKey); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				resetsAt = &t
			}
		}
	}

	return append(limits, UsageLimit{
		Name:           name,
		UtilizationPct: pct,
		ResetsAt:       resetsAt,
	})
}

func headerFloat(h http.Header, key string) float64 {
	v := h.Get(key)
	if v == "" {
		return 0
	}
	var f float64
	fmt.Sscanf(v, "%f", &f)
	return f
}
