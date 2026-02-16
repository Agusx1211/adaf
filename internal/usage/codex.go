package usage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type CodexProvider struct {
	warnThreshold     float64
	criticalThreshold float64
}

func NewCodexProvider(opts ...CodexOption) *CodexProvider {
	p := &CodexProvider{
		warnThreshold:     70.0,
		criticalThreshold: 90.0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

type CodexOption func(*CodexProvider)

func WithCodexThresholds(warn, critical float64) CodexOption {
	return func(p *CodexProvider) {
		p.warnThreshold = warn
		p.criticalThreshold = critical
	}
}

func (p *CodexProvider) Name() ProviderKind {
	return ProviderCodex
}

func (p *CodexProvider) HasCredentials() bool {
	return hasCodexChatGPTAuth() && findCodexBinary() != ""
}

func findCodexBinary() string {
	if path, err := exec.LookPath("codex"); err == nil {
		return path
	}
	return ""
}

func hasCodexChatGPTAuth() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	path := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var creds struct {
		AuthMode string `json:"auth_mode"`
	}

	if err := json.Unmarshal(data, &creds); err != nil {
		return false
	}

	return creds.AuthMode == "chatgpt"
}

type codexRateLimitsResponse struct {
	RateLimits          codexRateLimitSnapshot            `json:"rateLimits"`
	RateLimitsByLimitID map[string]codexRateLimitSnapshot `json:"rateLimitsByLimitId"`
}

type codexRateLimitSnapshot struct {
	LimitID   *string               `json:"limitId"`
	LimitName *string               `json:"limitName"`
	Primary   *codexRateLimitWindow `json:"primary"`
	Secondary *codexRateLimitWindow `json:"secondary"`
	Credits   *codexCreditsSnapshot `json:"credits"`
	PlanType  string                `json:"planType"`
}

type codexRateLimitWindow struct {
	UsedPercent        int   `json:"usedPercent"`
	WindowDurationMins int64 `json:"windowDurationMins"`
	ResetsAt           int64 `json:"resetsAt"`
}

type codexCreditsSnapshot struct {
	HasCredits bool   `json:"hasCredits"`
	Unlimited  bool   `json:"unlimited"`
	Balance    string `json:"balance"`
}

func (p *CodexProvider) FetchUsage(ctx context.Context) (UsageSnapshot, error) {
	binary := findCodexBinary()
	if binary == "" {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("codex binary not found in PATH"),
		}
	}

	cmd := exec.CommandContext(ctx, binary, "app-server")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = nil
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("failed to create stdout pipe: %w", err),
		}
	}
	cmd.Stderr = nil

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("failed to create stdin pipe: %w", err),
		}
	}

	if err := cmd.Start(); err != nil {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("failed to start codex app-server: %w", err),
		}
	}

	initMsg := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"clientInfo":{"name":"adaf","version":"0.1.0"},"protocolVersion":"2"}}` + "\n"
	rlMsg := `{"jsonrpc":"2.0","method":"account/rateLimits/read","id":2,"params":{}}` + "\n"

	if _, err := stdin.Write([]byte(initMsg)); err != nil {
		killCodexProcess(cmd)
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("failed to write initialize: %w", err),
		}
	}

	if _, err := stdin.Write([]byte(rlMsg)); err != nil {
		killCodexProcess(cmd)
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("failed to write rateLimits request: %w", err),
		}
	}
	stdin.Close()

	var rateLimits *codexRateLimitsResponse
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  interface{}     `json:"error"`
		}

		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.ID == 2 {
			if msg.Error != nil {
				cmd.Process.Kill()
				cmd.Wait()
				return UsageSnapshot{}, &ProviderError{
					Provider: ProviderCodex,
					Err:      fmt.Errorf("app-server error: %v", msg.Error),
				}
			}

			var resp codexRateLimitsResponse
			if err := json.Unmarshal(msg.Result, &resp); err != nil {
				cmd.Process.Kill()
				cmd.Wait()
				return UsageSnapshot{}, &ProviderError{
					Provider: ProviderCodex,
					Err:      fmt.Errorf("failed to parse rate limits: %w", err),
				}
			}
			rateLimits = &resp
			break
		}
	}

	killCodexProcess(cmd)

	if rateLimits == nil {
		return UsageSnapshot{}, &ProviderError{
			Provider: ProviderCodex,
			Err:      fmt.Errorf("no rate limit response from codex app-server"),
		}
	}

	var limits []UsageLimit

	if len(rateLimits.RateLimitsByLimitID) > 0 {
		for limitID, snap := range rateLimits.RateLimitsByLimitID {
			prefix := ""
			if limitID != "codex" {
				prefix = limitID + " "
			}

			if snap.Primary != nil {
				w := snap.Primary
				label := codexWindowLabel(w.WindowDurationMins, "Primary")
				if prefix != "" {
					label = prefix + label
				}
				limits = append(limits, UsageLimit{
					Name:           label,
					UtilizationPct: float64(w.UsedPercent),
					ResetsAt:       codexTimestampToTime(w.ResetsAt),
				})
			}

			if snap.Secondary != nil {
				w := snap.Secondary
				label := codexWindowLabel(w.WindowDurationMins, "Secondary")
				if prefix != "" {
					label = prefix + label
				}
				limits = append(limits, UsageLimit{
					Name:           label,
					UtilizationPct: float64(w.UsedPercent),
					ResetsAt:       codexTimestampToTime(w.ResetsAt),
				})
			}

			if snap.Credits != nil && !snap.Credits.Unlimited {
				c := snap.Credits
				balance := c.Balance
				if balance == "" {
					balance = "unknown"
				}
				util := 0.0
				if !c.HasCredits {
					util = 100.0
				}
				name := fmt.Sprintf("Credits ($%s)", balance)
				if prefix != "" {
					name = prefix + name
				}
				limits = append(limits, UsageLimit{
					Name:           name,
					UtilizationPct: util,
					ResetsAt:       nil,
				})
			}
		}
	} else {
		if rateLimits.RateLimits.Primary != nil {
			w := rateLimits.RateLimits.Primary
			label := codexWindowLabel(w.WindowDurationMins, "Primary")
			limits = append(limits, UsageLimit{
				Name:           label,
				UtilizationPct: float64(w.UsedPercent),
				ResetsAt:       codexTimestampToTime(w.ResetsAt),
			})
		}

		if rateLimits.RateLimits.Secondary != nil {
			w := rateLimits.RateLimits.Secondary
			label := codexWindowLabel(w.WindowDurationMins, "Secondary")
			limits = append(limits, UsageLimit{
				Name:           label,
				UtilizationPct: float64(w.UsedPercent),
				ResetsAt:       codexTimestampToTime(w.ResetsAt),
			})
		}

		if rateLimits.RateLimits.Credits != nil && !rateLimits.RateLimits.Credits.Unlimited {
			c := rateLimits.RateLimits.Credits
			balance := c.Balance
			if balance == "" {
				balance = "unknown"
			}
			util := 0.0
			if !c.HasCredits {
				util = 100.0
			}
			limits = append(limits, UsageLimit{
				Name:           fmt.Sprintf("Credits ($%s)", balance),
				UtilizationPct: util,
				ResetsAt:       nil,
			})
		}
	}

	return NewSnapshot(ProviderCodex, limits, p.warnThreshold, p.criticalThreshold), nil
}

func codexWindowLabel(mins int64, fallback string) string {
	switch {
	case mins >= 1440:
		return fmt.Sprintf("%d-day limit", mins/1440)
	case mins >= 60:
		return fmt.Sprintf("%d-hour limit", mins/60)
	case mins > 0:
		return fmt.Sprintf("%d-minute limit", mins)
	default:
		return fallback + " limit"
	}
}

func codexTimestampToTime(ts int64) *time.Time {
	if ts == 0 {
		return nil
	}
	t := time.Unix(ts, 0)
	return &t
}

func killCodexProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.Wait()
}

func IsCodexNotConfigured(err error) bool {
	if err == nil {
		return false
	}
	var pe *ProviderError
	if !strings.Contains(err.Error(), "codex binary not found") {
		return false
	}
	if errors.As(err, &pe) {
		return pe.Provider == ProviderCodex
	}
	return false
}
