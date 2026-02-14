package cli

import (
	"path/filepath"
	"strings"
)

// autoRegisterProject registers projectDir with the running daemon's project registry.
// Best-effort: returns silently on any error.
func autoRegisterProject(projectDir string) {
	cleanedDir := strings.TrimSpace(projectDir)
	if cleanedDir == "" {
		return
	}
	absDir, err := filepath.Abs(cleanedDir)
	if err != nil {
		return
	}
	absDir = filepath.Clean(absDir)

	state, running, err := loadWebDaemonState(webPIDFilePath(), webStateFilePath(), isPIDAlive)
	if err != nil || !running || state.PID == 0 {
		return
	}
	if !isPIDAlive(state.PID) {
		return
	}

	registryPath := webProjectsRegistryPath()
	registry, err := loadWebProjectRegistry(registryPath)
	if err != nil || registry == nil {
		return
	}

	project := webProjectRecord{
		ID:   projectIDFromPath(absDir),
		Path: absDir,
	}
	if !addWebProject(registry, project) {
		return
	}
	_ = saveWebProjectRegistry(registryPath, registry)
}
