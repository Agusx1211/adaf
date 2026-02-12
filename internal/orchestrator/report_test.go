package orchestrator

import "testing"

func TestExtractSpawnReport(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		expect  string
		wantErr bool
	}{
		{
			name:    "plain text without assistant is error",
			input:   "final plain output",
			wantErr: true,
		},
		{
			name: "vibe style role content",
			input: `{"role":"assistant","content":"draft"}
{"role":"assistant","content":""}
{"role":"assistant","content":"final report"}`,
			expect: "final report",
		},
		{
			name:   "claude style assistant blocks",
			input:  `{"type":"assistant","message":{"content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}]}}`,
			expect: "hello world",
		},
		{
			name: "codex item completed agent message",
			input: `{"type":"item.completed","item":{"type":"agent_message","text":"first"}}
{"type":"item.completed","item":{"type":"agent_message","text":"final"}}`,
			expect: "final",
		},
		{
			name: "json without assistant is error",
			input: `{"role":"tool","content":"tool output"}
{"role":"user","content":"prompt"}`,
			wantErr: true,
		},
		{
			name:    "empty output is error",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractSpawnReport(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("extractSpawnReport() error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("extractSpawnReport() unexpected error = %v", err)
			}
			if got != tt.expect {
				t.Fatalf("extractSpawnReport() = %q, want %q", got, tt.expect)
			}
		})
	}
}
