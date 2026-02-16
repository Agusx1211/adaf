package usage

import (
	"fmt"
	"testing"
	"time"
)

func TestProviderKindDisplayName(t *testing.T) {
	tests := []struct {
		provider ProviderKind
		want     string
	}{
		{ProviderClaude, "Claude Code"},
		{ProviderCodex, "Codex"},
		{ProviderGemini, "Gemini"},
		{ProviderKind("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			if got := tt.provider.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsageLevelString(t *testing.T) {
	tests := []struct {
		level UsageLevel
		want  string
	}{
		{LevelNormal, "normal"},
		{LevelWarning, "warning"},
		{LevelCritical, "critical"},
		{LevelExhausted, "exhausted"},
		{UsageLevel(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsageLimitRemainingPct(t *testing.T) {
	tests := []struct {
		name     string
		limit    UsageLimit
		expected float64
	}{
		{"empty", UsageLimit{UtilizationPct: 0}, 100},
		{"half", UsageLimit{UtilizationPct: 50}, 50},
		{"full", UsageLimit{UtilizationPct: 100}, 0},
		{"over", UsageLimit{UtilizationPct: 150}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.limit.RemainingPct(); got != tt.expected {
				t.Errorf("RemainingPct() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUsageLimitLevel(t *testing.T) {
	tests := []struct {
		name              string
		utilization       float64
		warnThreshold     float64
		criticalThreshold float64
		expected          UsageLevel
	}{
		{"normal", 50, 70, 90, LevelNormal},
		{"warning", 75, 70, 90, LevelWarning},
		{"critical", 95, 70, 90, LevelCritical},
		{"exhausted", 100, 70, 90, LevelExhausted},
		{"over", 150, 70, 90, LevelExhausted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := UsageLimit{UtilizationPct: tt.utilization}
			if got := limit.Level(tt.warnThreshold, tt.criticalThreshold); got != tt.expected {
				t.Errorf("Level() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUsageLimitTimeUntilReset(t *testing.T) {
	t.Run("nil resets_at", func(t *testing.T) {
		limit := UsageLimit{ResetsAt: nil}
		if got := limit.TimeUntilReset(); got != 0 {
			t.Errorf("TimeUntilReset() = %v, want 0", got)
		}
	})

	t.Run("future reset", func(t *testing.T) {
		future := time.Now().Add(5 * time.Minute)
		limit := UsageLimit{ResetsAt: &future}
		d := limit.TimeUntilReset()
		if d < 4*time.Minute || d > 5*time.Minute {
			t.Errorf("TimeUntilReset() = %v, want ~5m", d)
		}
	})

	t.Run("past reset", func(t *testing.T) {
		past := time.Now().Add(-5 * time.Minute)
		limit := UsageLimit{ResetsAt: &past}
		d := limit.TimeUntilReset()
		if d > 0 {
			t.Errorf("TimeUntilReset() = %v, want negative", d)
		}
	})
}

func TestNewSnapshot(t *testing.T) {
	limits := []UsageLimit{
		{Name: "5h", UtilizationPct: 50},
		{Name: "7d", UtilizationPct: 80},
	}

	snap := NewSnapshot(ProviderClaude, limits, 70, 90)

	if snap.Provider != ProviderClaude {
		t.Errorf("Provider = %v, want %v", snap.Provider, ProviderClaude)
	}

	if snap.OverallLevel != LevelWarning {
		t.Errorf("OverallLevel = %v, want %v", snap.OverallLevel, LevelWarning)
	}

	if len(snap.Limits) != 2 {
		t.Errorf("len(Limits) = %v, want 2", len(snap.Limits))
	}
}

func TestNewSnapshotEmptyLimits(t *testing.T) {
	snap := NewSnapshot(ProviderClaude, nil, 70, 90)

	if snap.OverallLevel != LevelNormal {
		t.Errorf("OverallLevel = %v, want %v", snap.OverallLevel, LevelNormal)
	}
}

func TestNewSnapshotMaxLevel(t *testing.T) {
	limits := []UsageLimit{
		{Name: "low", UtilizationPct: 10},
		{Name: "med", UtilizationPct: 50},
		{Name: "critical", UtilizationPct: 95},
	}

	snap := NewSnapshot(ProviderCodex, limits, 70, 90)

	if snap.OverallLevel != LevelCritical {
		t.Errorf("OverallLevel = %v, want %v", snap.OverallLevel, LevelCritical)
	}
}

func TestProviderError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		err := &ProviderError{
			Provider: ProviderClaude,
			Err:      nil,
		}
		if got := err.Error(); got != "Claude Code: unknown error" {
			t.Errorf("Error() = %q, want %q", got, "Claude Code: unknown error")
		}
		if err.Unwrap() != nil {
			t.Errorf("Unwrap() = %v, want nil", err.Unwrap())
		}
	})

	t.Run("with inner error", func(t *testing.T) {
		inner := fmt.Errorf("connection refused")
		err := &ProviderError{
			Provider: ProviderCodex,
			Err:      inner,
		}
		if got := err.Error(); got != "Codex: connection refused" {
			t.Errorf("Error() = %q, want %q", got, "Codex: connection refused")
		}
		if err.Unwrap() != inner {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), inner)
		}
	})
}
