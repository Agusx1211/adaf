package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type ClaudeProvider struct {
	oauthToken        string
	warnThreshold     float64
	criticalThreshold float64
}

func NewClaudeProvider(opts ...ClaudeOption) *ClaudeProvider {
	p := &ClaudeProvider{
		warnThreshold:     70.0,
		criticalThreshold: 90.0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

type ClaudeOption func(*ClaudeProvider)

func WithClaudeToken(token string) ClaudeOption {
	return func(p *ClaudeProvider) {
		p.oauthToken = token
	}
}

func WithClaudeThresholds(warn, critical float64) ClaudeOption {
	return func(p *ClaudeProvider) {
		p.warnThreshold = warn
		p.criticalThreshold = critical
	}
}

func (p *ClaudeProvider) Name() ProviderKind {
	return ProviderClaude
}

func (p *ClaudeProvider) HasCredentials() bool {
	_, err := p.getToken()
	return err == nil
}

func (p *ClaudeProvider) getToken() (string, error) {
	if p.oauthToken != "" {
		return p.oauthToken, nil
	}
	return getClaudeTokenFromDotfile()
}

func getClaudeTokenFromDotfile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	path := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}

	if err := json.Unmarshal(data, &creds); err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", path, err)
	}

	if creds.ClaudeAiOauth.AccessToken == "" {
		return "", fmt.Errorf("no claudeAiOauth.accessToken in %s", path)
	}

	return creds.ClaudeAiOauth.AccessToken, nil
}

type claudeUsageResponse struct {
	FiveHour     *claudeUsageWindow `json:"five_hour"`
	SevenDay     *claudeUsageWindow `json:"seven_day"`
	SevenDayOpus *claudeUsageWindow `json:"seven_day_opus"`
}

type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    *string `json:"resets_at"`
}

func (p *ClaudeProvider) FetchUsage(ctx context.Context) (UsageSnapshot, error) {
	token, err := p.getToken()
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderClaude, Err: err}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderClaude, Err: err}
	}

	req.SetBasicAuth(token, "")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UsageSnapshot{}, &ProviderError{Provider: ProviderClaude, Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderClaude,
			Err:      fmt.Errorf("OAuth token is invalid or expired"),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderClaude,
			Err:      fmt.Errorf("HTTP %d", resp.StatusCode),
		}
	}

	var usageResp claudeUsageResponse
	if err := json.NewDecoder(resp.Body).Decode(&usageResp); err != nil {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderClaude,
			Err:      fmt.Errorf("failed to parse response: %w", err),
		}
	}

	var limits []UsageLimit

	if usageResp.FiveHour != nil {
		limits = append(limits, UsageLimit{
			Name:           "5-hour window",
			UtilizationPct: usageResp.FiveHour.Utilization,
			ResetsAt:       parseClaudeTimestamp(usageResp.FiveHour.ResetsAt),
		})
	}

	if usageResp.SevenDay != nil {
		limits = append(limits, UsageLimit{
			Name:           "7-day window",
			UtilizationPct: usageResp.SevenDay.Utilization,
			ResetsAt:       parseClaudeTimestamp(usageResp.SevenDay.ResetsAt),
		})
	}

	if usageResp.SevenDayOpus != nil {
		limits = append(limits, UsageLimit{
			Name:           "7-day Opus",
			UtilizationPct: usageResp.SevenDayOpus.Utilization,
			ResetsAt:       parseClaudeTimestamp(usageResp.SevenDayOpus.ResetsAt),
		})
	}

	return NewSnapshot(ProviderClaude, limits, p.warnThreshold, p.criticalThreshold), nil
}

func parseClaudeTimestamp(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	return &t
}
