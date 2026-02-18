package failedcmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/agusx1211/adaf/internal/config"
)

func TestRecordUnknownCommandPersistsMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ADAF_PROFILE", "worker-codex")
	t.Setenv("ADAF_ROLE", "worker")
	t.Setenv("ADAF_POSITION", "worker")
	t.Setenv("ADAF_TURN_ID", "42")
	t.Setenv("ADAF_LOOP_RUN_ID", "7")
	t.Setenv("ADAF_PLAN_ID", "plan-main")
	t.Setenv("ADAF_PROJECT_DIR", "/tmp/demo-project")

	cfg := &config.GlobalConfig{
		Agents: map[string]config.GlobalAgentConfig{},
		Profiles: []config.Profile{
			{Name: "worker-codex", Agent: "codex", Model: "o3"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("config.Save() error = %v", err)
	}

	rec, path, err := Default().Record(errors.New(`unknown command "spwn" for "adaf"`), []string{"adaf", "spwn", "--task", "hello world"})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if rec == nil {
		t.Fatal("Record() returned nil record, want persisted record")
	}
	if rec.Kind != KindUnknownCommand {
		t.Fatalf("kind = %q, want %q", rec.Kind, KindUnknownCommand)
	}
	if rec.Profile != "worker-codex" {
		t.Fatalf("profile = %q, want %q", rec.Profile, "worker-codex")
	}
	if rec.Agent != "codex" {
		t.Fatalf("agent = %q, want %q", rec.Agent, "codex")
	}
	if rec.Model != "o3" {
		t.Fatalf("model = %q, want %q", rec.Model, "o3")
	}
	if rec.TurnID != "42" {
		t.Fatalf("turn_id = %q, want %q", rec.TurnID, "42")
	}
	if rec.Context["ADAF_TURN_ID"] != "42" {
		t.Fatalf("context ADAF_TURN_ID = %q, want %q", rec.Context["ADAF_TURN_ID"], "42")
	}
	if !strings.Contains(rec.Command, "hello world") {
		t.Fatalf("command = %q, want quoted arg with space", rec.Command)
	}
	wantDir := filepath.Join(config.Dir(), "failed-commands")
	if !strings.HasPrefix(path, wantDir+string(os.PathSeparator)) {
		t.Fatalf("path = %q, want prefix %q", path, wantDir)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var persisted Record
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if persisted.ID != rec.ID {
		t.Fatalf("persisted ID = %q, want %q", persisted.ID, rec.ID)
	}
	if persisted.Kind != KindUnknownCommand {
		t.Fatalf("persisted kind = %q, want %q", persisted.Kind, KindUnknownCommand)
	}
}

func TestClassify(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int("spawn-id", 0, "")
	parseErr := fs.Parse([]string{"--spawn-id", "abc"})
	if parseErr == nil {
		t.Fatal("expected parse error for invalid int flag")
	}

	tests := []struct {
		name string
		err  error
		want Kind
		ok   bool
	}{
		{
			name: "unknown command",
			err:  errors.New(`unknown command "spwn" for "adaf"`),
			want: KindUnknownCommand,
			ok:   true,
		},
		{
			name: "flag parse invalid value",
			err:  parseErr,
			want: KindInvalidArguments,
			ok:   true,
		},
		{
			name: "manual required flag style",
			err:  errors.New("--profile is required"),
			want: KindInvalidArguments,
			ok:   true,
		},
		{
			name: "runtime failure is not classified",
			err:  errors.New("spawn failed: daemon request failed"),
			want: "",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Classify(tt.err)
			if ok != tt.ok {
				t.Fatalf("Classify() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("Classify() kind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecordSkipsUnclassifiedErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec, path, err := Default().Record(errors.New("spawn failed: daemon request failed"), []string{"adaf", "spawn"})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if rec != nil {
		t.Fatalf("Record() record = %#v, want nil", rec)
	}
	if path != "" {
		t.Fatalf("Record() path = %q, want empty", path)
	}

	dir := filepath.Join(config.Dir(), "failed-commands")
	_, statErr := os.Stat(dir)
	if !os.IsNotExist(statErr) {
		t.Fatalf("failed-commands dir should not exist, stat err = %v", statErr)
	}
}
