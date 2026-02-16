package usage

import (
	"time"
)

type ProviderKind string

const (
	ProviderClaude ProviderKind = "claude"
	ProviderCodex  ProviderKind = "codex"
	ProviderGemini ProviderKind = "gemini"
)

func (p ProviderKind) String() string {
	return string(p)
}

func (p ProviderKind) DisplayName() string {
	switch p {
	case ProviderClaude:
		return "Claude Code"
	case ProviderCodex:
		return "Codex"
	case ProviderGemini:
		return "Gemini"
	default:
		return string(p)
	}
}

type UsageLevel int

const (
	LevelNormal UsageLevel = iota
	LevelWarning
	LevelCritical
	LevelExhausted
)

func (l UsageLevel) String() string {
	switch l {
	case LevelNormal:
		return "normal"
	case LevelWarning:
		return "warning"
	case LevelCritical:
		return "critical"
	case LevelExhausted:
		return "exhausted"
	default:
		return "unknown"
	}
}

type UsageLimit struct {
	Name           string
	UtilizationPct float64
	ResetsAt       *time.Time
}

func (l UsageLimit) RemainingPct() float64 {
	remaining := 100.0 - l.UtilizationPct
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (l UsageLimit) Level(warnThreshold, criticalThreshold float64) UsageLevel {
	if l.UtilizationPct >= 100 {
		return LevelExhausted
	}
	if l.UtilizationPct >= criticalThreshold {
		return LevelCritical
	}
	if l.UtilizationPct >= warnThreshold {
		return LevelWarning
	}
	return LevelNormal
}

func (l UsageLimit) TimeUntilReset() time.Duration {
	if l.ResetsAt == nil {
		return 0
	}
	return time.Until(*l.ResetsAt)
}

type UsageSnapshot struct {
	Provider     ProviderKind
	Timestamp    time.Time
	Limits       []UsageLimit
	OverallLevel UsageLevel
}

func NewSnapshot(provider ProviderKind, limits []UsageLimit, warnThreshold, criticalThreshold float64) UsageSnapshot {
	overall := LevelNormal
	for _, l := range limits {
		if level := l.Level(warnThreshold, criticalThreshold); level > overall {
			overall = level
		}
	}
	return UsageSnapshot{
		Provider:     provider,
		Timestamp:    time.Now(),
		Limits:       limits,
		OverallLevel: overall,
	}
}

type ProviderError struct {
	Provider ProviderKind
	Err      error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Provider.DisplayName() + ": " + e.Err.Error()
	}
	return e.Provider.DisplayName() + ": unknown error"
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}
