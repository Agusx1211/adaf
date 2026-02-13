package cli

func isTerminalSpawnStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "cancelled", "merged", "rejected":
		return true
	default:
		return false
	}
}
