package cli

import "testing"

func TestSessionsStopCommandRegistered(t *testing.T) {
	found := false
	for _, child := range sessionsCmd.Commands() {
		if child == sessionsStopCmd {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("sessions stop command is not registered under sessions command")
	}
}

func TestSessionsStopCommandFlags(t *testing.T) {
	if sessionsStopCmd.Flags().Lookup("force") == nil {
		t.Fatalf("expected --force flag to exist on sessions stop command")
	}

	if sessionsStopCmd.Flags().Lookup("all") == nil {
		t.Fatalf("expected --all flag to exist on sessions stop command")
	}
}
