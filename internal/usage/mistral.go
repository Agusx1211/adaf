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

type MistralProvider struct {
	apiKey            string
	warnThreshold     float64
	criticalThreshold float64
}

func NewMistralProvider(opts ...MistralOption) *MistralProvider {
	p := &MistralProvider{
		warnThreshold:     70.0,
		criticalThreshold: 90.0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

type MistralOption func(*MistralProvider)

func WithMistralAPIKey(key string) MistralOption {
	return func(p *MistralProvider) {
		p.apiKey = key
	}
}

func WithMistralThresholds(warn, critical float64) MistralOption {
	return func(p *MistralProvider) {
		p.warnThreshold = warn
		p.criticalThreshold = critical
	}
}

func (p *MistralProvider) Name() ProviderKind {
	return ProviderMistral
}

func (p *MistralProvider) HasCredentials() bool {
	key, err := p.getAPIKey()
	return err == nil && key != ""
}

func (p *MistralProvider) getAPIKey() (string, error) {
	if p.apiKey != "" {
		return p.apiKey, nil
	}

	if key := os.Getenv("MISTRAL_API_KEY"); key != "" {
		return key, nil
	}

	return getMistralKeyFromVibeEnv()
}

func getMistralKeyFromVibeEnv() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	path := filepath.Join(home, ".vibe", ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "MISTRAL_API_KEY=") {
			value := strings.TrimPrefix(line, "MISTRAL_API_KEY=")
			value = strings.TrimSpace(value)
			value = strings.Trim(value, `"'`)
			if value != "" {
				return value, nil
			}
		}
	}

	return "", fmt.Errorf("no MISTRAL_API_KEY found in %s", path)
}

func (p *MistralProvider) FetchUsage(ctx context.Context) (UsageSnapshot, error) {
	key, err := p.getAPIKey()
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderMistral, Err: err}
	}

	reqBody := map[string]interface{}{
		"model":      "mistral-small-latest",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mistral.ai/v1/chat/completions", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderMistral, Err: err}
	}

	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderMistral, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderMistral,
			Err:      fmt.Errorf("API key is invalid"),
		}
	}

	io.Copy(io.Discard, resp.Body)

	var limits []UsageLimit

	if limit, remaining := headerFloat(resp.Header, "x-ratelimit-limit-tokens-month"), headerFloat(resp.Header, "x-ratelimit-remaining-tokens-month"); limit > 0 {
		used := limit - remaining
		limits = append(limits, UsageLimit{
			Name:           "Monthly tokens",
			UtilizationPct: (used / limit) * 100,
		})
	}

	if limit, remaining := headerFloat(resp.Header, "x-ratelimit-limit-req-minute"), headerFloat(resp.Header, "x-ratelimit-remaining-req-minute"); limit > 0 {
		used := limit - remaining
		limits = append(limits, UsageLimit{
			Name:           "Req/min rate",
			UtilizationPct: (used / limit) * 100,
		})
	}

	if limit, remaining := headerFloat(resp.Header, "x-ratelimit-limit-tokens-minute"), headerFloat(resp.Header, "x-ratelimit-remaining-tokens-minute"); limit > 0 {
		used := limit - remaining
		limits = append(limits, UsageLimit{
			Name:           "Tokens/min rate",
			UtilizationPct: (used / limit) * 100,
		})
	}

	if len(limits) == 0 {
		limits = append(limits, UsageLimit{
			Name:           "Rate limit",
			UtilizationPct: 0,
		})
	}

	return NewSnapshot(ProviderMistral, limits, p.warnThreshold, p.criticalThreshold), nil
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
