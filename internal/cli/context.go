package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func turnIDFromEnv() (int, bool, error) {
	turnEnv := strings.TrimSpace(os.Getenv("ADAF_TURN_ID"))
	if turnEnv == "" {
		// Backward compat: fall back to ADAF_SESSION_ID
		turnEnv = strings.TrimSpace(os.Getenv("ADAF_SESSION_ID"))
	}
	if turnEnv == "" {
		return 0, false, nil
	}

	turnID, err := strconv.Atoi(turnEnv)
	if err != nil || turnID <= 0 {
		return 0, false, fmt.Errorf("invalid ADAF_TURN_ID: %q", turnEnv)
	}
	return turnID, true, nil
}

func resolveOptionalTurnID(turnFlag int) (int, error) {
	if turnFlag > 0 {
		return turnFlag, nil
	}
	turnID, found, err := turnIDFromEnv()
	if err != nil {
		return 0, err
	}
	if found {
		return turnID, nil
	}
	return 0, nil
}
