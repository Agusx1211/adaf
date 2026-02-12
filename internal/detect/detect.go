package detect

import (
	"context"
	"encoding/json"
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
	Name            string                   `json:"name"`
	Path            string                   `json:"path"`
	Version         string                   `json:"version"`
	Capabilities    []string                 `json:"capabilities,omitempty"`
	SupportedModels []string                 `json:"supported_models,omitempty"`
	ReasoningLevels []agentmeta.ReasoningLevel `json:"reasoning_levels,omitempty"`
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

	caps := append([]string(nil), meta.Capabilities...)
	models := append([]string(nil), meta.SupportedModels...)
	levels := append([]agentmeta.ReasoningLevel(nil), meta.ReasoningLevels...)

	// Probe for dynamic models and reasoning levels.
	probeModels, probeLevels := probeModelDiscovery(name, path)
	if len(probeModels) > 0 {
		// Dynamic results are authoritative — replace catalog models.
		models = probeModels
	}
	if len(probeLevels) > 0 {
		levels = probeLevels
	}

	return DetectedAgent{
		Name:            name,
		Path:            path,
		Version:         detectVersion(path),
		Capabilities:    caps,
		SupportedModels: models,
		ReasoningLevels: levels,
	}
}

// probeModelDiscovery dispatches per-agent dynamic model discovery.
func probeModelDiscovery(name, path string) (models []string, levels []agentmeta.ReasoningLevel) {
	switch name {
	case "codex":
		return probeCodexModels()
	case "claude":
		return probeClaudeModels(path)
	case "vibe":
		return probeVibeModels(), nil
	case "opencode":
		return probeOpencodeModels(), nil
	case "gemini":
		return probeGeminiModels()
	default:
		return nil, nil
	}
}

// --- Codex model discovery ---

// codexModelsCachePath returns ~/.codex/models_cache.json.
func codexModelsCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "models_cache.json")
}

// codexCacheFile represents the top-level structure of models_cache.json.
type codexCacheFile struct {
	Models []codexCacheModel `json:"models"`
}

// codexCacheModel represents an entry in the codex models_cache.json.
type codexCacheModel struct {
	Slug                     string                `json:"slug"`
	Visibility               string                `json:"visibility"`
	SupportedReasoningLevels []codexReasoningLevel `json:"supported_reasoning_levels"`
}

type codexReasoningLevel struct {
	Effort string `json:"effort"`
}

func probeCodexModels() ([]string, []agentmeta.ReasoningLevel) {
	cachePath := codexModelsCachePath()
	if cachePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, nil
	}

	var cacheFile codexCacheFile
	if err := json.Unmarshal(data, &cacheFile); err != nil {
		return nil, nil
	}
	entries := cacheFile.Models

	var models []string
	var levels []agentmeta.ReasoningLevel
	seenModels := make(map[string]struct{})
	levelsExtracted := false

	for _, e := range entries {
		if e.Visibility != "list" {
			continue
		}
		slug := strings.TrimSpace(e.Slug)
		if slug == "" {
			continue
		}
		lower := strings.ToLower(slug)
		if _, dup := seenModels[lower]; dup {
			continue
		}
		seenModels[lower] = struct{}{}
		models = append(models, slug)

		// Extract reasoning levels from the first visible model that has them.
		if !levelsExtracted && len(e.SupportedReasoningLevels) > 0 {
			for _, l := range e.SupportedReasoningLevels {
				effort := strings.TrimSpace(l.Effort)
				if effort != "" {
					levels = append(levels, agentmeta.ReasoningLevel{Name: effort})
				}
			}
			levelsExtracted = true
		}
	}

	return models, levels
}

// --- Claude model discovery ---

// claudeFullModelRE matches full model IDs like "claude-opus-4-6",
// "claude-sonnet-4-5-20250929", or "claude-opus-4-6-v1" in the CLI bundle.
// These are the precise identifiers the CLI accepts via --model.
var claudeFullModelRE = regexp.MustCompile(`"(claude-(?:opus|sonnet|haiku)[-0-9a-z]*)"`)

// claudeAliasRE matches bare alias references like "opus", "sonnet", "haiku",
// "best", "opusplan" (without any version suffix) which the CLI also accepts
// as shortcuts for the latest.
var claudeAliasRE = regexp.MustCompile(`"((?:opus|sonnet|haiku|best|opusplan))"`)

// claudeEffortRE matches the effort levels array, e.g. ["low","medium","high","max"]
var claudeEffortRE = regexp.MustCompile(`\["low","medium","high"(?:,"max")?\]`)

func probeClaudeModels(binPath string) ([]string, []agentmeta.ReasoningLevel) {
	// The claude binary is typically a symlink into an npm package containing cli.js.
	// Follow symlinks to find the package directory.
	cliJS := resolveClaudeCLIJS(binPath)
	if cliJS == "" {
		return nil, nil
	}

	data, err := os.ReadFile(cliJS)
	if err != nil {
		return nil, nil
	}

	// The CLI accepts two forms via --model:
	//   1. Bare aliases: "opus", "sonnet", "haiku" (resolve to latest version)
	//   2. Full IDs: "claude-opus-4-6", "claude-sonnet-4-5-20250929", etc.
	// We extract both from the bundle. Bare aliases go first (most useful),
	// then full IDs for when the user wants a specific version.
	seen := make(map[string]struct{})
	var models []string

	add := func(name string) {
		lower := strings.ToLower(name)
		if _, dup := seen[lower]; dup {
			return
		}
		seen[lower] = struct{}{}
		models = append(models, name)
	}

	// Bare aliases first.
	for _, m := range claudeAliasRE.FindAllSubmatch(data, -1) {
		add(string(m[1]))
	}

	// Full model IDs.
	for _, m := range claudeFullModelRE.FindAllSubmatch(data, -1) {
		add(string(m[1]))
	}

	// Extract effort/reasoning levels from the bundle.
	var levels []agentmeta.ReasoningLevel
	if effortMatch := claudeEffortRE.Find(data); effortMatch != nil {
		var raw []string
		if json.Unmarshal(effortMatch, &raw) == nil {
			for _, l := range raw {
				levels = append(levels, agentmeta.ReasoningLevel{Name: l})
			}
		}
	}

	return models, levels
}

// resolveClaudeCLIJS attempts to find the cli.js file from the claude binary path.
//
// The claude binary is typically either:
//  1. A symlink into an npm/bun package's cli.js (e.g. ~/.bun/bin/claude -> .../node_modules/@anthropic-ai/claude-code/cli.js)
//  2. A shell wrapper that invokes cli.js
//  3. A native binary (in which case cli.js is in the same directory)
func resolveClaudeCLIJS(binPath string) string {
	// Resolve symlinks to find the real binary location.
	resolved, err := filepath.EvalSymlinks(binPath)
	if err != nil {
		resolved = binPath
	}

	// Check if the resolved path itself IS cli.js (common with bun/npm installs
	// where the binary is a symlink directly to cli.js).
	if filepath.Base(resolved) == "cli.js" {
		if fi, err := os.Stat(resolved); err == nil && !fi.IsDir() {
			return resolved
		}
	}

	dir := filepath.Dir(resolved)

	// Try common locations relative to the resolved binary.
	candidates := []string{
		filepath.Join(dir, "cli.js"),
		filepath.Join(dir, "..", "lib", "cli.js"),
		filepath.Join(dir, "..", "dist", "cli.js"),
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}

	return ""
}

// --- Vibe model discovery ---

// vibeModelAliasRE extracts alias values from [[models]] sections in config.toml.
var vibeModelAliasRE = regexp.MustCompile(`(?m)^alias\s*=\s*"([^"]+)"`)

func probeVibeModels() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(home, ".vibe", "config.toml"))
	if err != nil {
		return nil
	}

	matches := vibeModelAliasRE.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	var models []string
	for _, m := range matches {
		alias := strings.TrimSpace(string(m[1]))
		if alias == "" {
			continue
		}
		lower := strings.ToLower(alias)
		if _, dup := seen[lower]; dup {
			continue
		}
		seen[lower] = struct{}{}
		models = append(models, alias)
	}
	return models
}

// --- OpenCode model discovery ---

// opencodeConfig represents the relevant parts of ~/.config/opencode/opencode.json.
type opencodeConfig struct {
	Provider map[string]opencodeProvider `json:"provider"`
}

type opencodeProvider struct {
	Models map[string]opencodeModel `json:"models"`
}

type opencodeModel struct {
	Name string `json:"name"`
}

func probeOpencodeModels() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "opencode.json"))
	if err != nil {
		return nil
	}

	var cfg opencodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var models []string

	for providerID, provider := range cfg.Provider {
		for modelID := range provider.Models {
			// Use provider/model format for custom providers.
			qualified := providerID + "/" + modelID
			lower := strings.ToLower(qualified)
			if _, dup := seen[lower]; dup {
				continue
			}
			seen[lower] = struct{}{}
			models = append(models, qualified)
		}
	}

	return models
}

// --- Gemini model discovery ---

// geminiSettings represents the relevant parts of ~/.gemini/settings.json.
type geminiSettings struct {
	Model string `json:"model"`
}

func probeGeminiModels() ([]string, []agentmeta.ReasoningLevel) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil
	}

	data, err := os.ReadFile(filepath.Join(home, ".gemini", "settings.json"))
	if err != nil {
		return nil, nil
	}

	var settings geminiSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, nil
	}

	if settings.Model == "" {
		return nil, nil
	}

	// Start with the configured model, then add defaults if different.
	seen := make(map[string]struct{})
	var models []string
	add := func(m string) {
		lower := strings.ToLower(m)
		if _, dup := seen[lower]; dup {
			return
		}
		seen[lower] = struct{}{}
		models = append(models, m)
	}

	add(settings.Model)

	// Always include the well-known models.
	for _, m := range []string{"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash"} {
		add(m)
	}

	return models, nil
}

// --- Helpers ---

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
