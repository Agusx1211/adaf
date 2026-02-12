package agent

import "testing"

func TestModelRegistry(t *testing.T) {
	if got := DefaultModel("claude"); got != "sonnet" {
		t.Fatalf("DefaultModel(claude) = %q", got)
	}

	models := SupportedModels("codex")
	if len(models) == 0 {
		t.Fatal("expected codex model list to be non-empty")
	}

	if !IsModelSupported("codex", "o4-mini") {
		t.Fatal("expected o4-mini to be supported for codex")
	}
	if IsModelSupported("codex", "not-a-model") {
		t.Fatal("unexpected unsupported codex model accepted")
	}
	if !IsModelSupported("opencode", "any/provider-model") {
		t.Fatal("expected opencode to allow provider-qualified model")
	}
}
