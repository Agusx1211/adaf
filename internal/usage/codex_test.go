package usage

import (
	"encoding/json"
	"testing"
)

func TestCodexMultiLimitParsing(t *testing.T) {
	jsonData := `{
		"rateLimits": {
			"limitId": "codex",
			"primary": {
				"usedPercent": 15,
				"windowDurationMins": 300,
				"resetsAt": 1770617566
			},
			"secondary": {
				"usedPercent": 55,
				"windowDurationMins": 10080,
				"resetsAt": 1770718715
			},
			"credits": {
				"hasCredits": true,
				"unlimited": false,
				"balance": "1000"
			},
			"planType": "pro"
		},
		"rateLimitsByLimitId": {
			"codex": {
				"limitId": "codex",
				"primary": {
					"usedPercent": 15,
					"windowDurationMins": 300,
					"resetsAt": 1770617566
				},
				"secondary": {
					"usedPercent": 55,
					"windowDurationMins": 10080,
					"resetsAt": 1770718715
				},
				"credits": {
					"hasCredits": true,
					"unlimited": false,
					"balance": "1000"
				},
				"planType": "pro"
			},
			"codex_spark": {
				"limitId": "codex_spark",
				"limitName": "Spark",
				"primary": {
					"usedPercent": 30,
					"windowDurationMins": 60,
					"resetsAt": 1770618000
				},
				"planType": "pro"
			}
		}
	}`

	var resp codexRateLimitsResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.RateLimits.Primary == nil {
		t.Error("Expected primary to be non-nil")
	}

	if len(resp.RateLimitsByLimitID) != 2 {
		t.Errorf("Expected 2 limits in map, got %d", len(resp.RateLimitsByLimitID))
	}

	codex, ok := resp.RateLimitsByLimitID["codex"]
	if !ok {
		t.Error("Expected 'codex' key in rate limits map")
	}
	if codex.Primary == nil || codex.Primary.UsedPercent != 15 {
		t.Errorf("Expected codex primary usedPercent=15, got %v", codex.Primary)
	}

	spark, ok := resp.RateLimitsByLimitID["codex_spark"]
	if !ok {
		t.Error("Expected 'codex_spark' key in rate limits map")
	}
	if spark.Primary == nil || spark.Primary.UsedPercent != 30 {
		t.Errorf("Expected spark primary usedPercent=30, got %v", spark.Primary)
	}
	if spark.Primary.WindowDurationMins != 60 {
		t.Errorf("Expected spark window duration=60, got %d", spark.Primary.WindowDurationMins)
	}
}

func TestCodexSingleLimitParsing(t *testing.T) {
	jsonData := `{
		"rateLimits": {
			"limitId": "codex",
			"primary": {
				"usedPercent": 15,
				"windowDurationMins": 300,
				"resetsAt": 1770617566
			},
			"planType": "pro"
		}
	}`

	var resp codexRateLimitsResponse
	if err := json.Unmarshal([]byte(jsonData), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.RateLimits.Primary == nil {
		t.Error("Expected primary to be non-nil")
	}

	if resp.RateLimits.Primary.UsedPercent != 15 {
		t.Errorf("Expected usedPercent=15, got %d", resp.RateLimits.Primary.UsedPercent)
	}

	if len(resp.RateLimitsByLimitID) > 0 {
		t.Errorf("Expected empty rate limits map for single limit response, got %d", len(resp.RateLimitsByLimitID))
	}
}
