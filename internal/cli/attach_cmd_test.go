package cli

import "testing"

func TestAttachCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"attach"})
	if err != nil {
		t.Fatalf("rootCmd.Find(attach): %v", err)
	}
	if cmd == nil || cmd.Name() != "attach" {
		t.Fatalf("attach command not registered, got %v", cmd)
	}
}

func TestAttachCommandHasJSONFlag(t *testing.T) {
	flag := attachCmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatalf("attach --json flag not found")
	}
	if flag.DefValue != "false" {
		t.Fatalf("attach --json default = %q, want false", flag.DefValue)
	}
}
