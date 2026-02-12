// Package debug provides a verbose structured logger for development diagnostics.
//
// When enabled via --debug, every significant event in the ADAF runtime is
// written to a single .log file under ~/.adaf/debug/. The log includes
// nanosecond timestamps, goroutine IDs, caller locations, and all relevant
// context IDs (turn, spawn, session, loop run, step, hex IDs) so that any
// execution path can be reconstructed after the fact.
//
// When disabled (the default), all logging functions are no-ops with zero
// allocation overhead.
package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agusx1211/adaf/internal/hexid"
)

// logger is the global debug logger. nil when debug mode is off.
var (
	logger   *Logger
	loggerMu sync.RWMutex
)

// Logger writes structured debug lines to a file.
type Logger struct {
	mu        sync.Mutex
	file      *os.File
	path      string
	startedAt time.Time
}

// Init initializes the global debug logger. It creates ~/.adaf/debug/ if
// needed and opens a log file named with the current timestamp and a random
// hex ID. Returns the log file path. Calling Init when debug mode is off
// is unnecessary — all Log/Logf calls are no-ops when the logger is nil.
func Init() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("debug: user home dir: %w", err)
	}

	dir := filepath.Join(home, ".adaf", "debug")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("debug: create dir %s: %w", dir, err)
	}

	now := time.Now()
	hid := hexid.New()
	filename := fmt.Sprintf("%s_%s.log", now.Format("20060102T150405"), hid)
	path := filepath.Join(dir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", fmt.Errorf("debug: open log %s: %w", path, err)
	}

	l := &Logger{
		file:      f,
		path:      path,
		startedAt: now,
	}

	// Write header.
	header := fmt.Sprintf(
		"=== ADAF DEBUG LOG ===\nStarted: %s\nPID: %d\nGOMAXPROCS: %d\nLog ID: %s\nFile: %s\n===\n\n",
		now.Format(time.RFC3339Nano),
		os.Getpid(),
		runtime.GOMAXPROCS(0),
		hid,
		path,
	)
	f.WriteString(header)

	loggerMu.Lock()
	logger = l
	loggerMu.Unlock()

	return path, nil
}

// Close flushes and closes the debug log. Safe to call when not initialized.
func Close() {
	loggerMu.Lock()
	l := logger
	logger = nil
	loggerMu.Unlock()

	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	elapsed := time.Since(l.startedAt)
	l.file.WriteString(fmt.Sprintf("\n=== DEBUG LOG CLOSED === (duration=%s)\n", elapsed))
	l.file.Close()
}

// Enabled returns true if the debug logger is active.
func Enabled() bool {
	loggerMu.RLock()
	e := logger != nil
	loggerMu.RUnlock()
	return e
}

// Path returns the log file path, or "" if not enabled.
func Path() string {
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	if l == nil {
		return ""
	}
	return l.path
}

// Log writes a debug line. No-op when debug is disabled.
// The line is prefixed with a nanosecond timestamp, goroutine ID, and caller.
func Log(component, msg string) {
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	if l == nil {
		return
	}
	l.write(component, msg, 2)
}

// Logf writes a formatted debug line. No-op when debug is disabled.
func Logf(component, format string, args ...any) {
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	if l == nil {
		return
	}
	l.write(component, fmt.Sprintf(format, args...), 2)
}

// LogKV writes a debug line with key-value context pairs.
// Usage: debug.LogKV("loop", "turn started", "turn_id", 5, "hex_id", "ab12cd34")
func LogKV(component, msg string, kvs ...any) {
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	if l == nil {
		return
	}

	var b strings.Builder
	b.WriteString(msg)
	for i := 0; i+1 < len(kvs); i += 2 {
		b.WriteString(fmt.Sprintf(" %v=%v", kvs[i], kvs[i+1]))
	}
	l.write(component, b.String(), 2)
}

// write formats and appends a single log line.
func (l *Logger) write(component, msg string, callerSkip int) {
	now := time.Now()
	elapsed := now.Sub(l.startedAt)

	// Get goroutine ID from the stack (cheap enough for debug mode).
	gid := goroutineID()

	// Caller info.
	_, file, line, ok := runtime.Caller(callerSkip)
	caller := "??:0"
	if ok {
		// Shorten to package/file.go:line
		if idx := strings.LastIndex(file, "/internal/"); idx >= 0 {
			file = file[idx+1:]
		} else if idx := strings.LastIndex(file, "/cmd/"); idx >= 0 {
			file = file[idx+1:]
		} else if idx := strings.LastIndex(file, "/pkg/"); idx >= 0 {
			file = file[idx+1:]
		}
		caller = fmt.Sprintf("%s:%d", file, line)
	}

	// Format: TIMESTAMP +ELAPSED [GID] [COMPONENT] CALLER | MESSAGE
	line2 := fmt.Sprintf("%s +%12s [G%-6d] [%-14s] %-40s | %s\n",
		now.Format("15:04:05.000000000"),
		elapsed.Truncate(time.Microsecond),
		gid,
		component,
		caller,
		msg,
	)

	l.mu.Lock()
	l.file.WriteString(line2)
	l.mu.Unlock()
}

// goroutineID extracts the goroutine ID from runtime.Stack output.
// This is intentionally used only in debug mode where performance is secondary.
func goroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := string(buf[:n])
	// Format: "goroutine 123 [..."
	if !strings.HasPrefix(s, "goroutine ") {
		return 0
	}
	s = s[len("goroutine "):]
	var id int64
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + int64(c-'0')
	}
	return id
}
