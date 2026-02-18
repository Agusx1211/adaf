package failedcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/config"
	"github.com/agusx1211/adaf/internal/hexid"
)

const (
	defaultDirName = "failed-commands"
	schemaVersion  = 1
)

// Kind identifies the reason category for a failed command attempt.
type Kind string

const (
	KindUnknownCommand   Kind = "unknown_command"
	KindInvalidArguments      = "invalid_arguments"
)

// Recorder writes command failure records to disk.
type Recorder struct {
	dir string
}

// Record captures one failed command attempt with runtime context.
type Record struct {
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	RecordedAt time.Time `json:"recorded_at"`

	Kind  Kind   `json:"kind"`
	Error string `json:"error"`

	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	Command    string   `json:"command"`

	WorkingDir string `json:"working_dir,omitempty"`

	Profile  string `json:"profile,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Model    string `json:"model,omitempty"`
	Role     string `json:"role,omitempty"`
	Position string `json:"position,omitempty"`

	ProjectDir    string `json:"project_dir,omitempty"`
	PlanID        string `json:"plan_id,omitempty"`
	TurnID        string `json:"turn_id,omitempty"`
	TurnHexID     string `json:"turn_hex_id,omitempty"`
	ParentTurnID  string `json:"parent_turn_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	LoopRunID     string `json:"loop_run_id,omitempty"`
	LoopStepIndex string `json:"loop_step_index,omitempty"`
	LoopRunHexID  string `json:"loop_run_hex_id,omitempty"`
	LoopStepHexID string `json:"loop_step_hex_id,omitempty"`

	Context map[string]string `json:"context,omitempty"`
}

// Default returns a recorder rooted at ~/.adaf/failed-commands.
func Default() *Recorder {
	return &Recorder{dir: filepath.Join(config.Dir(), defaultDirName)}
}

// New returns a recorder rooted at dir.
func New(dir string) *Recorder {
	return &Recorder{dir: strings.TrimSpace(dir)}
}

// Dir returns the configured output directory.
func (r *Recorder) Dir() string {
	if r == nil {
		return ""
	}
	return r.dir
}

// Record classifies err and, when relevant, writes one JSON record to disk.
// Returns (nil, "", nil) when err is not a tracked command failure class.
func (r *Recorder) Record(err error, argv []string) (*Record, string, error) {
	kind, ok := Classify(err)
	if !ok {
		return nil, "", nil
	}

	rec := buildRecord(kind, err, argv)
	path, writeErr := r.write(rec)
	if writeErr != nil {
		return nil, "", writeErr
	}
	return rec, path, nil
}

// Classify reports whether err is an unknown command or invalid-argument failure.
func Classify(err error) (Kind, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, pflag.ErrHelp) {
		return "", false
	}

	var notExistErr *pflag.NotExistError
	if errors.As(err, &notExistErr) {
		return KindInvalidArguments, true
	}
	var valueRequiredErr *pflag.ValueRequiredError
	if errors.As(err, &valueRequiredErr) {
		return KindInvalidArguments, true
	}
	var invalidValueErr *pflag.InvalidValueError
	if errors.As(err, &invalidValueErr) {
		return KindInvalidArguments, true
	}
	var invalidSyntaxErr *pflag.InvalidSyntaxError
	if errors.As(err, &invalidSyntaxErr) {
		return KindInvalidArguments, true
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return "", false
	}

	if strings.Contains(msg, "unknown command ") && strings.Contains(msg, ` for "adaf`) {
		return KindUnknownCommand, true
	}
	if isInvalidArgumentMessage(msg) {
		return KindInvalidArguments, true
	}

	return "", false
}

func isInvalidArgumentMessage(msg string) bool {
	// Cobra/pflag messages.
	if strings.Contains(msg, "unknown flag:") ||
		strings.Contains(msg, "unknown shorthand flag:") ||
		strings.Contains(msg, "flag needs an argument:") ||
		strings.Contains(msg, "invalid argument ") ||
		strings.Contains(msg, "bad flag syntax:") ||
		strings.Contains(msg, "required flag(s)") ||
		strings.Contains(msg, "no such flag -") ||
		strings.Contains(msg, "at least one of the flags in the group [") ||
		strings.Contains(msg, "if any flags in the group [") {
		return true
	}

	if strings.Contains(msg, "arg(s), received") &&
		(strings.Contains(msg, "accepts ") || strings.Contains(msg, "requires at least ")) {
		return true
	}

	// Common adaf command-level validation styles.
	if strings.HasPrefix(msg, "--") {
		if strings.Contains(msg, " is required") ||
			strings.Contains(msg, " are required") ||
			strings.Contains(msg, " must be") ||
			strings.Contains(msg, " cannot be") ||
			strings.Contains(msg, "cannot be combined") ||
			strings.Contains(msg, "mutually exclusive") ||
			strings.Contains(msg, "invalid --") {
			return true
		}
	}

	if strings.HasPrefix(msg, "invalid ") {
		if strings.Contains(msg, "must be a number") ||
			strings.Contains(msg, "(valid:") {
			return true
		}
	}

	return false
}

func buildRecord(kind Kind, err error, argv []string) *Record {
	now := time.Now().UTC()
	if len(argv) == 0 {
		argv = append([]string(nil), os.Args...)
	} else {
		argv = append([]string(nil), argv...)
	}

	ctx := collectAdafEnv()
	profile := strings.TrimSpace(ctx["ADAF_PROFILE"])
	agentName := strings.TrimSpace(ctx["ADAF_AGENT_NAME"])
	modelName := strings.TrimSpace(ctx["ADAF_MODEL"])

	if agentName == "" || modelName == "" {
		if inferredAgent, inferredModel := inferAgentAndModel(profile); inferredAgent != "" || inferredModel != "" {
			if agentName == "" {
				agentName = inferredAgent
			}
			if modelName == "" {
				modelName = inferredModel
			}
		}
	}

	rec := &Record{
		Version:    schemaVersion,
		ID:         fmt.Sprintf("%s-%s", now.Format("20060102T150405.000000000Z"), hexid.New()),
		RecordedAt: now,
		Kind:       kind,
		Error:      strings.TrimSpace(err.Error()),

		Executable: firstArg(argv),
		Args:       argsWithoutExecutable(argv),
		Command:    formatCommand(argv),

		Profile:  profile,
		Agent:    agentName,
		Model:    modelName,
		Role:     strings.TrimSpace(ctx["ADAF_ROLE"]),
		Position: strings.TrimSpace(ctx["ADAF_POSITION"]),

		ProjectDir:    strings.TrimSpace(ctx["ADAF_PROJECT_DIR"]),
		PlanID:        strings.TrimSpace(ctx["ADAF_PLAN_ID"]),
		TurnID:        strings.TrimSpace(ctx["ADAF_TURN_ID"]),
		TurnHexID:     strings.TrimSpace(ctx["ADAF_TURN_HEX_ID"]),
		ParentTurnID:  strings.TrimSpace(ctx["ADAF_PARENT_TURN"]),
		SessionID:     strings.TrimSpace(ctx["ADAF_SESSION_ID"]),
		LoopRunID:     strings.TrimSpace(ctx["ADAF_LOOP_RUN_ID"]),
		LoopStepIndex: strings.TrimSpace(ctx["ADAF_LOOP_STEP_INDEX"]),
		LoopRunHexID:  strings.TrimSpace(ctx["ADAF_LOOP_RUN_HEX_ID"]),
		LoopStepHexID: strings.TrimSpace(ctx["ADAF_LOOP_STEP_HEX_ID"]),

		Context: ctx,
	}

	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		rec.WorkingDir = cwd
	}

	return rec
}

func (r *Recorder) write(rec *Record) (string, error) {
	if rec == nil {
		return "", fmt.Errorf("failed command record is nil")
	}
	if r == nil || strings.TrimSpace(r.dir) == "" {
		return "", fmt.Errorf("failed command output dir is empty")
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return "", fmt.Errorf("creating failed command dir: %w", err)
	}

	name := fmt.Sprintf("%s-%d-%s.json",
		rec.RecordedAt.UTC().Format("20060102T150405.000000000Z"),
		os.Getpid(),
		hexid.New(),
	)
	path := filepath.Join(r.dir, name)
	tmp := path + ".tmp"

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding failed command record: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("writing failed command temp record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("replacing failed command record: %w", err)
	}

	return path, nil
}

func collectAdafEnv() map[string]string {
	ctx := make(map[string]string)
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if !strings.HasPrefix(key, "ADAF_") {
			continue
		}
		ctx[key] = value
	}
	if len(ctx) == 0 {
		return nil
	}
	return ctx
}

func inferAgentAndModel(profileName string) (string, string) {
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return "", ""
	}

	cfg, err := config.Load()
	if err == nil && cfg != nil {
		if prof := cfg.FindProfile(profileName); prof != nil {
			agentName := strings.TrimSpace(prof.Agent)
			modelName := strings.TrimSpace(prof.Model)
			if modelName == "" && agentName != "" {
				modelName = strings.TrimSpace(agent.DefaultModel(agentName))
			}
			return agentName, modelName
		}
	}

	candidate := inferAgentFromProfileName(profileName)
	if candidate == "" {
		return "", ""
	}
	return candidate, strings.TrimSpace(agent.DefaultModel(candidate))
}

func inferAgentFromProfileName(profileName string) string {
	profileName = strings.ToLower(strings.TrimSpace(profileName))
	if profileName == "" {
		return ""
	}
	if _, ok := agent.Get(profileName); ok {
		return profileName
	}
	if _, tail, ok := strings.Cut(profileName, ":"); ok {
		tail = strings.TrimSpace(tail)
		if _, exists := agent.Get(tail); exists {
			return tail
		}
	}
	return ""
}

func firstArg(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	return argv[0]
}

func argsWithoutExecutable(argv []string) []string {
	if len(argv) <= 1 {
		return nil
	}
	return append([]string(nil), argv[1:]...)
}

func formatCommand(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(argv))
	for _, arg := range argv {
		parts = append(parts, quoteShellArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteShellArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if strings.ContainsAny(arg, " \t\n\"'\\$") {
		return strconv.Quote(arg)
	}
	return arg
}
