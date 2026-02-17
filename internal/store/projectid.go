package store

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type projectMarker struct {
	ID string `json:"id"`
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

// ProjectMarkerPath returns the marker file path (<projectDir>/.adaf.json).
func ProjectMarkerPath(projectDir string) string {
	return filepath.Join(cleanPath(projectDir), ProjectMarkerFile)
}

// FindProjectDir walks up from startDir until a directory containing
// .adaf.json is found. It returns an empty string when no marker is present.
func FindProjectDir(startDir string) (string, error) {
	candidate := cleanPath(startDir)
	for {
		markerPath := ProjectMarkerPath(candidate)
		if _, err := os.Stat(markerPath); err == nil {
			return candidate, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		candidate = parent
	}
	return "", nil
}

// ReadProjectID reads the project id from <projectDir>/.adaf.json.
func ReadProjectID(projectDir string) (string, error) {
	path := ProjectMarkerPath(projectDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var marker projectMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return "", fmt.Errorf("parsing %s: %w", path, err)
	}
	id := strings.TrimSpace(marker.ID)
	if id == "" {
		return "", fmt.Errorf("parsing %s: missing id", path)
	}
	return id, nil
}

func writeProjectMarker(projectDir, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return fmt.Errorf("project id is empty")
	}
	projectDir = cleanPath(projectDir)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projectMarker{ID: projectID}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ProjectMarkerPath(projectDir), data, 0644)
}

func projectsRootDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".adaf", "projects")
}

// ProjectStoreDirForID returns ~/.adaf/projects/<id>.
func ProjectStoreDirForID(projectID string) string {
	return filepath.Join(projectsRootDir(), strings.TrimSpace(projectID))
}

// GenerateProjectID returns an id in the format "<readable>-<uuid-v4>".
func GenerateProjectID(projectDir string) (string, error) {
	base := sanitizeForProjectID(filepath.Base(cleanPath(projectDir)))
	if base == "" {
		base = "project"
	}
	uuid, err := randomUUIDv4()
	if err != nil {
		return "", err
	}
	return base + "-" + uuid, nil
}

// ProjectIDFromDir returns the marker id when present, or a deterministic
// fallback derived from the directory path.
func ProjectIDFromDir(projectDir string) string {
	if projectID, err := ReadProjectID(projectDir); err == nil && strings.TrimSpace(projectID) != "" {
		return projectID
	}
	return fallbackProjectID(projectDir)
}

func fallbackProjectID(projectDir string) string {
	abs := cleanPath(projectDir)
	base := sanitizeForProjectID(filepath.Base(abs))
	if base == "" {
		base = "project"
	}
	sum := sha1.Sum([]byte(abs))
	hash := hex.EncodeToString(sum[:])[:8]
	return base + "-" + hash
}

func sanitizeForProjectID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	prevDash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func randomUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	hexStr := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hexStr[0:8],
		hexStr[8:12],
		hexStr[12:16],
		hexStr[16:20],
		hexStr[20:32],
	), nil
}
