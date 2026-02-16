package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/agusx1211/adaf/internal/agent"
	"github.com/agusx1211/adaf/internal/store"
)

func TestClassifySpawnCompletion(t *testing.T) {
	t.Run("canceled preserves child exit code", func(t *testing.T) {
		status, exitCode, result := classifySpawnCompletion(context.Canceled, &agent.Result{ExitCode: -1})
		if status != "canceled" {
			t.Fatalf("status = %q, want canceled", status)
		}
		if exitCode != -1 {
			t.Fatalf("exit_code = %d, want -1", exitCode)
		}
		if result != "" {
			t.Fatalf("result = %q, want empty", result)
		}
	})

	t.Run("wrapped canceled is treated as canceled", func(t *testing.T) {
		status, exitCode, result := classifySpawnCompletion(errors.Join(errors.New("wrapper"), context.Canceled), &agent.Result{ExitCode: -1})
		if status != "canceled" {
			t.Fatalf("status = %q, want canceled", status)
		}
		if exitCode != -1 {
			t.Fatalf("exit_code = %d, want -1", exitCode)
		}
		if result != "" {
			t.Fatalf("result = %q, want empty", result)
		}
	})

	t.Run("canceled without child result uses sentinel exit code", func(t *testing.T) {
		status, exitCode, result := classifySpawnCompletion(context.Canceled, nil)
		if status != "canceled" {
			t.Fatalf("status = %q, want canceled", status)
		}
		if exitCode != -1 {
			t.Fatalf("exit_code = %d, want -1", exitCode)
		}
		if result != "" {
			t.Fatalf("result = %q, want empty", result)
		}
	})

	t.Run("failure keeps child exit code", func(t *testing.T) {
		runErr := errors.New("boom")
		status, exitCode, result := classifySpawnCompletion(runErr, &agent.Result{ExitCode: 7})
		if status != "failed" {
			t.Fatalf("status = %q, want failed", status)
		}
		if exitCode != 7 {
			t.Fatalf("exit_code = %d, want 7", exitCode)
		}
		if result != runErr.Error() {
			t.Fatalf("result = %q, want %q", result, runErr.Error())
		}
	})

	t.Run("failure without child result defaults to exit code 1", func(t *testing.T) {
		runErr := errors.New("boom")
		status, exitCode, result := classifySpawnCompletion(runErr, nil)
		if status != "failed" {
			t.Fatalf("status = %q, want failed", status)
		}
		if exitCode != 1 {
			t.Fatalf("exit_code = %d, want 1", exitCode)
		}
		if result != runErr.Error() {
			t.Fatalf("result = %q, want %q", result, runErr.Error())
		}
	})

	t.Run("successful run keeps completed status", func(t *testing.T) {
		status, exitCode, result := classifySpawnCompletion(nil, &agent.Result{ExitCode: 0})
		if status != "completed" {
			t.Fatalf("status = %q, want completed", status)
		}
		if exitCode != 0 {
			t.Fatalf("exit_code = %d, want 0", exitCode)
		}
		if result != "" {
			t.Fatalf("result = %q, want empty", result)
		}
	})
}

func TestCanceledSpawnMessage(t *testing.T) {
	got := canceledSpawnMessage(true)
	want := "Spawn was canceled before completion. Partial work was auto-committed."
	if got != want {
		t.Fatalf("canceledSpawnMessage(true) = %q, want %q", got, want)
	}

	got = canceledSpawnMessage(false)
	want = "Spawn was canceled before completion."
	if got != want {
		t.Fatalf("canceledSpawnMessage(false) = %q, want %q", got, want)
	}
}

func TestAppendSpawnSummary(t *testing.T) {
	if got := appendSpawnSummary("", "note"); got != "note" {
		t.Fatalf("appendSpawnSummary(empty) = %q, want %q", got, "note")
	}

	got := appendSpawnSummary("child report", "canceled")
	want := "child report\n\ncanceled"
	if got != want {
		t.Fatalf("appendSpawnSummary(joined) = %q, want %q", got, want)
	}
}

func TestIsTerminalSpawnStatusIncludesCanceled(t *testing.T) {
	if !store.IsTerminalSpawnStatus("canceled") {
		t.Fatalf("store.IsTerminalSpawnStatus(canceled) = false, want true")
	}
	if !store.IsTerminalSpawnStatus("cancelled") {
		t.Fatalf("store.IsTerminalSpawnStatus(cancelled) = false, want true")
	}
}
