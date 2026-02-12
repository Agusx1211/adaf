package session

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestClassifySessionEnd(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus string
		wantMsg    string
	}{
		{
			name:       "done",
			err:        nil,
			wantStatus: "done",
			wantMsg:    "",
		},
		{
			name:       "cancelled direct",
			err:        context.Canceled,
			wantStatus: "cancelled",
			wantMsg:    "",
		},
		{
			name:       "cancelled wrapped",
			err:        fmt.Errorf("agent run failed: %w", context.Canceled),
			wantStatus: "cancelled",
			wantMsg:    "",
		},
		{
			name:       "error",
			err:        errors.New("boom"),
			wantStatus: "error",
			wantMsg:    "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMsg := classifySessionEnd(tt.err)
			if gotStatus != tt.wantStatus {
				t.Fatalf("status = %q, want %q", gotStatus, tt.wantStatus)
			}
			if gotMsg != tt.wantMsg {
				t.Fatalf("err msg = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestDonePayloadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "cancelled direct", err: context.Canceled, want: ""},
		{name: "cancelled wrapped", err: fmt.Errorf("wrap: %w", context.Canceled), want: ""},
		{name: "error", err: errors.New("failed"), want: "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := donePayloadError(tt.err)
			if got != tt.want {
				t.Fatalf("donePayloadError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeDaemonExit(t *testing.T) {
	if err := normalizeDaemonExit(context.Canceled); err != nil {
		t.Fatalf("normalizeDaemonExit(context.Canceled) = %v, want nil", err)
	}

	wrapped := fmt.Errorf("wrapped: %w", context.Canceled)
	if err := normalizeDaemonExit(wrapped); err != nil {
		t.Fatalf("normalizeDaemonExit(wrapped canceled) = %v, want nil", err)
	}

	expected := errors.New("boom")
	if err := normalizeDaemonExit(expected); !errors.Is(err, expected) {
		t.Fatalf("normalizeDaemonExit(non-cancelled) = %v, want %v", err, expected)
	}
}
