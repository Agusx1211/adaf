package cli

import "testing"

func TestHealthCheckArgsModelOverrideBehavior(t *testing.T) {
	t.Run("codex without override omits model flag", func(t *testing.T) {
		args := healthCheckArgs("codex", "")
		for i := range args {
			if args[i] == "--model" {
				t.Fatalf("unexpected --model in args: %v", args)
			}
		}
	})

	t.Run("codex with override includes model flag", func(t *testing.T) {
		args := healthCheckArgs("codex", "o3")
		if len(args) < 3 {
			t.Fatalf("expected model args, got %v", args)
		}
		if args[0] != "--model" || args[1] != "o3" {
			t.Fatalf("unexpected model args: %v", args)
		}
	})
}
