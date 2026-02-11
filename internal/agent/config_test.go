package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agusx1211/adaf/internal/detect"
)

func TestSyncDetectedAgentsPersistsAndPreservesOverrides(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".adaf")

	cfg, err := SyncDetectedAgents(root, []detect.DetectedAgent{
		{
			Name:            "claude",
			Path:            "/tmp/claude",
			Version:         "1.0.0",
			Capabilities:    []string{"prompt-arg"},
			SupportedModels: []string{"opus-4", "sonnet-4.5"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SyncDetectedAgents() error = %v", err)
	}

	rec, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatal("expected claude to be present")
	}
	if !rec.Detected {
		t.Fatal("expected claude to be marked detected")
	}
	if rec.DefaultModel != "sonnet-4.5" {
		t.Fatalf("unexpected default model: %q", rec.DefaultModel)
	}

	rec.ModelOverride = "opus-4"
	rec.DefaultModel = "opus-4"
	cfg.Agents["claude"] = rec
	if err := SaveAgentsConfig(root, cfg); err != nil {
		t.Fatalf("SaveAgentsConfig() error = %v", err)
	}

	cfg, err = SyncDetectedAgents(root, []detect.DetectedAgent{
		{
			Name:            "claude",
			Path:            "/tmp/claude2",
			Version:         "1.1.0",
			Capabilities:    []string{"prompt-arg", "model-select"},
			SupportedModels: []string{"opus-4", "sonnet-4.5", "haiku-4.5"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("SyncDetectedAgents() second call error = %v", err)
	}

	rec = cfg.Agents["claude"]
	if rec.Path != "/tmp/claude2" {
		t.Fatalf("expected updated path, got %q", rec.Path)
	}
	if rec.Version != "1.1.0" {
		t.Fatalf("expected updated version, got %q", rec.Version)
	}
	if rec.ModelOverride != "opus-4" {
		t.Fatalf("expected override preserved, got %q", rec.ModelOverride)
	}
	if rec.DefaultModel != "opus-4" {
		t.Fatalf("expected effective default to keep override, got %q", rec.DefaultModel)
	}

	if _, err := os.Stat(AgentsConfigPath(root)); err != nil {
		t.Fatalf("expected agents config file to exist: %v", err)
	}
}

func TestResolveDefaultModel(t *testing.T) {
	cfg := &AgentsConfig{
		Agents: map[string]AgentRecord{
			"codex": {
				Name:          "codex",
				ModelOverride: "o3",
			},
		},
	}

	if got := ResolveDefaultModel(cfg, nil, "codex"); got != "o3" {
		t.Fatalf("ResolveDefaultModel(codex) = %q", got)
	}
	if got := ResolveDefaultModel(nil, nil, "claude"); got != "sonnet-4.5" {
		t.Fatalf("ResolveDefaultModel(nil, claude) = %q", got)
	}
}

func TestResolveModelOverride(t *testing.T) {
	cfg := &AgentsConfig{
		Agents: map[string]AgentRecord{
			"codex": {
				Name:          "codex",
				ModelOverride: "gpt-4.1",
				DefaultModel:  "o4-mini",
			},
			"claude": {
				Name:         "claude",
				DefaultModel: "sonnet-4.5",
			},
		},
	}

	if got := ResolveModelOverride(cfg, nil, "codex"); got != "gpt-4.1" {
		t.Fatalf("ResolveModelOverride(codex) = %q", got)
	}
	if got := ResolveModelOverride(cfg, nil, "claude"); got != "" {
		t.Fatalf("ResolveModelOverride(claude) = %q, want empty", got)
	}
	if got := ResolveModelOverride(nil, nil, "codex"); got != "" {
		t.Fatalf("ResolveModelOverride(nil, codex) = %q, want empty", got)
	}
}
