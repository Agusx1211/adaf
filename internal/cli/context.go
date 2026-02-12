package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func sessionIDFromEnv() (int, bool, error) {
	sessionEnv := strings.TrimSpace(os.Getenv("ADAF_SESSION_ID"))
	if sessionEnv == "" {
		return 0, false, nil
	}

	sessionID, err := strconv.Atoi(sessionEnv)
	if err != nil || sessionID <= 0 {
		return 0, false, fmt.Errorf("invalid ADAF_SESSION_ID: %q", sessionEnv)
	}
	return sessionID, true, nil
}

func resolveOptionalSessionID(sessionFlag int) (int, error) {
	if sessionFlag > 0 {
		return sessionFlag, nil
	}
	sessionID, found, err := sessionIDFromEnv()
	if err != nil {
		return 0, err
	}
	if found {
		return sessionID, nil
	}
	return 0, nil
}

func resolveRequiredSessionID(sessionFlag int) (int, error) {
	sessionID, err := resolveOptionalSessionID(sessionFlag)
	if err != nil {
		return 0, err
	}
	if sessionID <= 0 {
		return 0, fmt.Errorf("--session is required (or run inside an adaf agent session)")
	}
	return sessionID, nil
}
