package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckWebDaemon(t *testing.T) {
	// Setup temp home
	tempHome := t.TempDir()
	
	var envVar string
	if runtime.GOOS == "windows" {
		envVar = "USERPROFILE"
	} else {
		envVar = "HOME"
	}
	
	oldHome := os.Getenv(envVar)
	os.Setenv(envVar, tempHome)
	defer os.Setenv(envVar, oldHome)

	m := &AppModel{}

	// 1. No file exists
	m.checkWebDaemon()
	if m.webServerURL != "" {
		t.Errorf("expected empty webServerURL, got %q", m.webServerURL)
	}

	// 2. Valid file, use current PID which is definitely alive
	adafDir := filepath.Join(tempHome, ".adaf")
	if err := os.MkdirAll(adafDir, 0755); err != nil {
		t.Fatalf("failed to create adaf dir: %v", err)
	}
	
	myPID := os.Getpid()
	info := struct {
		PID int    `json:"pid"`
		URL string `json:"url"`
	}{
		PID: myPID,
		URL: "http://localhost:8080",
	}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(adafDir, "web.json"), data, 0644); err != nil {
		t.Fatalf("failed to write web.json: %v", err)
	}

	m.checkWebDaemon()
	if m.webServerURL != "http://localhost:8080" {
		t.Errorf("expected http://localhost:8080, got %q", m.webServerURL)
	}

	// 3. Invalid PID (likely doesn't exist)
	info.PID = 9999999 
	data, _ = json.Marshal(info)
	if err := os.WriteFile(filepath.Join(adafDir, "web.json"), data, 0644); err != nil {
		t.Fatalf("failed to write web.json: %v", err)
	}
	
	m.checkWebDaemon()
	if m.webServerURL != "" {
		t.Errorf("expected empty webServerURL for dead PID, got %q", m.webServerURL)
	}
}
