package usage

import (
	"net/http"
	"testing"
)

func TestMistralProviderDisplayName(t *testing.T) {
	p := NewMistralProvider()
	if p.Name() != ProviderMistral {
		t.Errorf("Name() = %v, want %v", p.Name(), ProviderMistral)
	}
}

func TestOpenAIProviderDisplayName(t *testing.T) {
	p := NewOpenAIProvider()
	if p.Name() != ProviderOpenAI {
		t.Errorf("Name() = %v, want %v", p.Name(), ProviderOpenAI)
	}
}

func TestMistralProviderWithOptions(t *testing.T) {
	p := NewMistralProvider(
		WithMistralAPIKey("test-key"),
		WithMistralThresholds(60, 80),
	)
	if p.apiKey != "test-key" {
		t.Errorf("apiKey = %v, want test-key", p.apiKey)
	}
	if p.warnThreshold != 60 {
		t.Errorf("warnThreshold = %v, want 60", p.warnThreshold)
	}
	if p.criticalThreshold != 80 {
		t.Errorf("criticalThreshold = %v, want 80", p.criticalThreshold)
	}
}

func TestOpenAIProviderWithOptions(t *testing.T) {
	p := NewOpenAIProvider(
		WithOpenAIAPIKey("test-key"),
		WithOpenAIBaseURL("https://custom.api.com/v1"),
		WithOpenAIThresholds(50, 75),
	)
	if p.apiKey != "test-key" {
		t.Errorf("apiKey = %v, want test-key", p.apiKey)
	}
	if p.baseURL != "https://custom.api.com/v1" {
		t.Errorf("baseURL = %v, want https://custom.api.com/v1", p.baseURL)
	}
	if p.warnThreshold != 50 {
		t.Errorf("warnThreshold = %v, want 50", p.warnThreshold)
	}
}

func TestHeaderFloat(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		key     string
		want    float64
	}{
		{"present", http.Header{"X-Limit": []string{"1000"}}, "X-Limit", 1000},
		{"missing", http.Header{}, "X-Limit", 0},
		{"with decimal", http.Header{"X-Limit": []string{"12.5"}}, "X-Limit", 12.5},
		{"invalid", http.Header{"X-Limit": []string{"abc"}}, "X-Limit", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerFloat(tt.headers, tt.key)
			if got != tt.want {
				t.Errorf("headerFloat() = %v, want %v", got, tt.want)
			}
		})
	}
}
