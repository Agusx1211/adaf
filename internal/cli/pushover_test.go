package cli

import "testing"

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{name: "empty", input: "", output: ""},
		{name: "short", input: "abc", output: "***"},
		{name: "exactly four", input: "abcd", output: "****"},
		{name: "longer than four", input: "abcdefgh", output: "abcd****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskSecret(tt.input); got != tt.output {
				t.Fatalf("maskSecret(%q) = %q, want %q", tt.input, got, tt.output)
			}
		})
	}
}
