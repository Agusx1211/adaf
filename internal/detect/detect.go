package detect

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agusx1211/adaf/internal/agentmeta"
)

const (
	versionProbeTimeout = 1800 * time.Millisecond
)

var semverRE = regexp.MustCompile(`(?i)\bv?(\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.-]+)?)\b`)

// DetectedAgent describes an installed agent tool discovered on the machine.
type DetectedAgent struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Version         string   `json:"version"`
	Capabilities    []string `json:"capabilities,omitempty"`
	SupportedModels []string `json:"supported_models,omitempty"`
}

// Scan discovers installed agent CLIs from PATH and known install locations.
func Scan() ([]DetectedAgent, error) {
	found := make(map[string]DetectedAgent)
	seenPaths := make(map[string]struct{})

	known := knownBinaryCandidates()
	for name, binaries := range known {
		for _, bin := range binaries {
			path, ok := resolveBinaryPath(bin)
			if !ok {
				continue
			}
			if _, exists := seenPaths[path]; exists {
				continue
			}
			found[name] = buildDetected(name, path)
			seenPaths[path] = struct{}{}
			break
		}
	}

	// Explicit generic candidates from curated list and environment.
	for _, candidate := range genericNameCandidates() {
		if _, exists := found[candidate]; exists {
			continue
		}
		path, ok := resolveBinaryPath(candidate)
		if !ok {
			continue
		}
		if _, exists := seenPaths[path]; exists {
			continue
		}
		name := normalizeName(candidate)
		if isKnownAgent(name) {
			continue
		}
		found[name] = buildDetected(name, path)
		seenPaths[path] = struct{}{}
	}

	// Heuristic PATH scan for generic agent tools.
	for _, item := range scanGenericFromPATH(found, seenPaths) {
		found[item.Name] = item
		seenPaths[item.Path] = struct{}{}
	}

	agents := make([]DetectedAgent, 0, len(found))
	for _, d := range found {
		agents = append(agents, d)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })

	return agents, nil
}

func buildDetected(name, path string) DetectedAgent {
	meta, ok := agentmeta.InfoFor(name)
	if !ok {
		meta, _ = agentmeta.InfoFor("generic")
	}

	return DetectedAgent{
		Name:            name,
		Path:            path,
		Version:         detectVersion(path),
		Capabilities:    append([]string(nil), meta.Capabilities...),
		SupportedModels: append([]string(nil), meta.SupportedModels...),
	}
}

func knownBinaryCandidates() map[string][]string {
	known := make(map[string][]string)
	for name, bin := range agentmeta.BinaryNames() {
		known[name] = []string{bin}
	}
	// Common alternative binary names.
	known["opencode"] = append(known["opencode"], "opencode-cli")
	return known
}

func genericNameCandidates() []string {
	candidates := []string{
		"aider",
		"goose",
		"continue",
		"cursor-agent",
		"roo",
		"cody",
		"llm",
		"qwen-code",
		"gemini",
		"chatgpt",
	}

	for _, v := range strings.Split(os.Getenv("ADAF_EXTRA_AGENT_BINS"), ",") {
		v = normalizeName(v)
		if v != "" {
			candidates = append(candidates, v)
		}
	}

	uniq := make(map[string]struct{}, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		c = normalizeName(c)
		if c == "" {
			continue
		}
		if _, exists := uniq[c]; exists {
			continue
		}
		uniq[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func resolveBinaryPath(binary string) (string, bool) {
	candidates := make([]string, 0, 1+len(knownInstallDirs()))
	if p, err := exec.LookPath(binary); err == nil {
		candidates = append(candidates, p)
	}

	for _, dir := range knownInstallDirs() {
		candidates = append(candidates, filepath.Join(dir, binary))
	}

	for _, path := range candidates {
		if real, ok := executablePath(path); ok {
			return real, true
		}
	}

	return "", false
}

func knownInstallDirs() []string {
	dirs := []string{
		"/usr/local/bin",
		"/usr/bin",
		"/opt/homebrew/bin",
		"/opt/local/bin",
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, "bin"),
			filepath.Join(home, ".npm-global", "bin"),
		)
	}

	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "Programs"))
		}
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			dirs = append(dirs, pf)
		}
	}

	uniq := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if _, exists := uniq[dir]; exists {
			continue
		}
		uniq[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}

func executablePath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(strings.ToLower(path), ".exe") {
			if _, err := os.Stat(path + ".exe"); err == nil {
				path += ".exe"
			}
		}
	}

	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return "", false
	}
	if runtime.GOOS != "windows" && fi.Mode()&0111 == 0 {
		return "", false
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		abs = resolved
	}
	return abs, true
}

func detectVersion(commandPath string) string {
	attempts := [][]string{{"--version"}, {"-v"}, {"version"}}

	for _, args := range attempts {
		out, err := runVersionProbe(commandPath, args)
		if err != nil && out == "" {
			continue
		}
		if version := parseVersion(out); version != "" {
			return version
		}
	}

	return "unknown"
}

func runVersionProbe(commandPath string, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, commandPath, args...)
	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, ctx.Err()
	}
	return out, err
}

func parseVersion(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	if matches := semverRE.FindStringSubmatch(output); len(matches) > 1 {
		return matches[1]
	}

	line := output
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if len(line) > 48 {
		line = line[:48]
	}
	return line
}

func scanGenericFromPATH(found map[string]DetectedAgent, seenPaths map[string]struct{}) []DetectedAgent {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}

	knownNames := make(map[string]struct{}, len(found)+8)
	for name := range found {
		knownNames[name] = struct{}{}
	}
	for name := range knownBinaryCandidates() {
		knownNames[name] = struct{}{}
	}

	results := make([]DetectedAgent, 0, 8)
	seenNames := make(map[string]struct{})

	for _, dir := range filepath.SplitList(pathEnv) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}

			name := normalizeName(e.Name())
			if name == "" || !looksLikeGenericAgentName(name) {
				continue
			}
			if _, exists := knownNames[name]; exists {
				continue
			}
			if _, exists := seenNames[name]; exists {
				continue
			}

			path, ok := executablePath(filepath.Join(dir, e.Name()))
			if !ok {
				continue
			}
			if _, exists := seenPaths[path]; exists {
				continue
			}

			results = append(results, buildDetected(name, path))
			seenNames[name] = struct{}{}

			if len(results) >= 12 {
				return results
			}
		}
	}

	return results
}

func looksLikeGenericAgentName(name string) bool {
	name = normalizeName(name)
	if name == "" {
		return false
	}

	if _, banned := genericNameBlocklist[name]; banned {
		return false
	}

	for _, token := range genericNameTokens {
		if hasNameToken(name, token) {
			return true
		}
	}
	return false
}

func hasNameToken(name, token string) bool {
	if name == token {
		return true
	}
	for start := strings.Index(name, token); start >= 0; {
		end := start + len(token)
		prevBoundary := start == 0 || !isAlphaNum(rune(name[start-1]))
		nextBoundary := end == len(name) || !isAlphaNum(rune(name[end]))
		if prevBoundary || nextBoundary {
			return true
		}
		next := strings.Index(name[end:], token)
		if next < 0 {
			break
		}
		start = end + next
	}
	return false
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

var genericNameTokens = []string{
	"aider",
	"goose",
	"cursor",
	"copilot",
	"cody",
	"llm",
	"gpt",
	"gemini",
	"qwen",
	"chatgpt",
	"openai",
}

var genericNameBlocklist = map[string]struct{}{
	"ssh-agent":                      {},
	"gpg-agent":                      {},
	"pkttyagent":                     {},
	"systemd-tty-ask-password-agent": {},
}

func normalizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, ".exe")
	}
	return name
}

func isKnownAgent(name string) bool {
	_, ok := agentmeta.InfoFor(name)
	return ok && name != "generic"
}
