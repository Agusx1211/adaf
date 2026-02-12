package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/stream"
)

var sessionsOutputCmd = &cobra.Command{
	Use:     "output [session-id|profile]",
	Aliases: []string{"out", "tail"},
	Short:   "Show session output without attaching",
	Long: `Stream or inspect session output without opening the interactive attach UI.

By default this command shows output for running sessions. Use --agent to filter
by agent type (for example: codex, claude, vibe) and --all to include completed
sessions.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionsOutput,
}

var sessionsDaemonLogsCmd = &cobra.Command{
	Use:     "logs [session-id|profile]",
	Aliases: []string{"log", "daemon-logs", "adaf-logs"},
	Short:   "Show adaf daemon logs for sessions",
	Long: `Show adaf runtime logs (daemon stdout/stderr) for one or more sessions.

This is useful for debugging daemon-level failures that are not agent output.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSessionsDaemonLogs,
}

func init() {
	sessionsOutputCmd.Flags().String("agent", "", "Filter sessions by agent name")
	sessionsOutputCmd.Flags().Bool("all", false, "Include completed/dead sessions when no session is specified")
	sessionsOutputCmd.Flags().BoolP("follow", "f", false, "Follow new output as it arrives")
	sessionsOutputCmd.Flags().IntP("lines", "n", 200, "Show last N lines per session (0 = all lines)")
	sessionsOutputCmd.Flags().Bool("raw", false, "Print raw wire JSON instead of formatted output")
	sessionsCmd.AddCommand(sessionsOutputCmd)

	sessionsDaemonLogsCmd.Flags().String("agent", "", "Filter sessions by agent name")
	sessionsDaemonLogsCmd.Flags().Bool("all", false, "Include completed/dead sessions when no session is specified")
	sessionsDaemonLogsCmd.Flags().BoolP("follow", "f", false, "Follow new log lines as they are written")
	sessionsDaemonLogsCmd.Flags().IntP("lines", "n", 200, "Show last N log lines per session (0 = all lines)")
	sessionsCmd.AddCommand(sessionsDaemonLogsCmd)
}

func runSessionsOutput(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("session management is not available inside an agent context")
	}

	agentFilter, _ := cmd.Flags().GetString("agent")
	includeAll, _ := cmd.Flags().GetBool("all")
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")
	raw, _ := cmd.Flags().GetBool("raw")
	out := cmd.OutOrStdout()
	if lines < 0 {
		return fmt.Errorf("--lines must be >= 0")
	}

	query := ""
	if len(args) == 1 {
		query = strings.TrimSpace(args[0])
	}

	targets, err := selectSessionTargets(query, agentFilter, includeAll)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, colorDim+"  No matching sessions."+colorReset)
		return nil
	}

	multi := len(targets) > 1
	followTargets := make([]fileFollowTarget, 0, len(targets))

	for i := range targets {
		meta := targets[i]
		prefix := ""
		if multi {
			prefix = fmt.Sprintf("[session:%d %s] ", meta.ID, meta.AgentName)
		} else {
			fmt.Fprintf(out, "  %sSession #%d (%s / %s)%s\n", colorDim, meta.ID, meta.ProfileName, meta.AgentName, colorReset)
		}

		writer := io.Writer(out)
		if prefix != "" {
			writer = newPrefixedLineWriter(out, prefix)
		}
		renderer := newSessionOutputRenderer(writer, raw)

		eventsPath := session.EventsPath(meta.ID)
		err := replayFileLines(eventsPath, lines, renderer.RenderLine)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(out, "  %sNo output file yet for session #%d.%s\n", colorDim, meta.ID, colorReset)
			} else {
				return fmt.Errorf("reading session %d output: %w", meta.ID, err)
			}
		}
		renderer.Finish()

		if follow {
			r := renderer
			followTargets = append(followTargets, fileFollowTarget{
				Path:   eventsPath,
				Offset: fileSize(eventsPath),
				Render: r.RenderLine,
				Finish: r.Finish,
			})
		}
	}

	if !follow {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return followFiles(ctx, followTargets)
}

func runSessionsDaemonLogs(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("session management is not available inside an agent context")
	}

	agentFilter, _ := cmd.Flags().GetString("agent")
	includeAll, _ := cmd.Flags().GetBool("all")
	follow, _ := cmd.Flags().GetBool("follow")
	lines, _ := cmd.Flags().GetInt("lines")
	out := cmd.OutOrStdout()
	if lines < 0 {
		return fmt.Errorf("--lines must be >= 0")
	}

	query := ""
	if len(args) == 1 {
		query = strings.TrimSpace(args[0])
	}

	targets, err := selectSessionTargets(query, agentFilter, includeAll)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Fprintln(out, colorDim+"  No matching sessions."+colorReset)
		return nil
	}

	multi := len(targets) > 1
	followTargets := make([]fileFollowTarget, 0, len(targets))

	for i := range targets {
		meta := targets[i]
		prefix := ""
		if multi {
			prefix = fmt.Sprintf("[session:%d daemon] ", meta.ID)
		} else {
			fmt.Fprintf(out, "  %sSession #%d daemon log%s\n", colorDim, meta.ID, colorReset)
		}

		writer := io.Writer(out)
		if prefix != "" {
			writer = newPrefixedLineWriter(out, prefix)
		}

		render := func(line []byte) error {
			_, err := writer.Write(line)
			return err
		}

		logPath := session.DaemonLogPath(meta.ID)
		err := replayFileLines(logPath, lines, render)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(out, "  %sNo daemon log yet for session #%d.%s\n", colorDim, meta.ID, colorReset)
			} else {
				return fmt.Errorf("reading session %d daemon logs: %w", meta.ID, err)
			}
		}

		if follow {
			followTargets = append(followTargets, fileFollowTarget{
				Path:   logPath,
				Offset: fileSize(logPath),
				Render: render,
			})
		}
	}

	if !follow {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return followFiles(ctx, followTargets)
}

func selectSessionTargets(query, agentFilter string, includeAll bool) ([]session.SessionMeta, error) {
	agentFilter = strings.TrimSpace(strings.ToLower(agentFilter))

	if query != "" {
		meta, err := session.FindSessionByPartial(query)
		if err != nil {
			return nil, err
		}
		if !matchesAgentFilter(meta.AgentName, agentFilter) {
			return nil, fmt.Errorf("session %d uses agent %q (does not match --agent %q)", meta.ID, meta.AgentName, agentFilter)
		}
		return []session.SessionMeta{*meta}, nil
	}

	sessions, err := session.ListSessions()
	if err != nil {
		return nil, err
	}

	var out []session.SessionMeta
	for _, s := range sessions {
		if !includeAll && s.Status != "running" && s.Status != "starting" {
			continue
		}
		if !matchesAgentFilter(s.AgentName, agentFilter) {
			continue
		}
		out = append(out, s)
	}

	return out, nil
}

func matchesAgentFilter(agentName, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(agentName), filter)
}

// sessionOutputRenderer decodes wire messages and prints formatted agent output.
type sessionOutputRenderer struct {
	out     io.Writer
	raw     bool
	display *stream.Display
}

func newSessionOutputRenderer(out io.Writer, raw bool) *sessionOutputRenderer {
	r := &sessionOutputRenderer{
		out: out,
		raw: raw,
	}
	if !raw {
		r.display = stream.NewDisplay(out)
	}
	return r
}

func (r *sessionOutputRenderer) RenderLine(line []byte) error {
	if len(bytes.TrimSpace(line)) == 0 {
		return nil
	}

	if r.raw {
		_, err := r.out.Write(line)
		return err
	}

	msg, err := session.DecodeMsg(bytes.TrimSpace(line))
	if err != nil {
		return nil
	}

	switch msg.Type {
	case session.MsgRaw:
		data, err := session.DecodeData[session.WireRaw](msg)
		if err != nil || data == nil || data.Data == "" {
			return nil
		}
		_, err = io.WriteString(r.out, data.Data)
		return err

	case session.MsgEvent:
		data, err := session.DecodeData[session.WireEvent](msg)
		if err != nil || data == nil || len(data.Event) == 0 {
			return nil
		}
		var ev stream.ClaudeEvent
		if err := json.Unmarshal(data.Event, &ev); err != nil {
			return nil
		}
		r.display.Handle(ev)
		return nil

	case session.MsgFinished:
		data, err := session.DecodeData[session.WireFinished](msg)
		if err != nil || data == nil {
			return nil
		}
		_, err = fmt.Fprintf(r.out, "%s[finished]%s exit=%d duration=%s\n",
			colorDim, colorReset, data.ExitCode, time.Duration(data.DurationNS).Round(time.Second))
		return err

	case session.MsgDone:
		data, err := session.DecodeData[session.WireDone](msg)
		if err != nil || data == nil {
			return nil
		}
		if data.Error != "" {
			_, err = fmt.Fprintf(r.out, "%s[done] error:%s %s\n", colorRed, colorReset, data.Error)
		} else {
			_, err = fmt.Fprintf(r.out, "%s[done]%s\n", colorDim, colorReset)
		}
		return err
	}

	return nil
}

func (r *sessionOutputRenderer) Finish() {
	if r.display != nil {
		r.display.Finish()
	}
}

type prefixedLineWriter struct {
	w           io.Writer
	prefix      string
	atLineStart bool
}

func newPrefixedLineWriter(w io.Writer, prefix string) *prefixedLineWriter {
	return &prefixedLineWriter{
		w:           w,
		prefix:      prefix,
		atLineStart: true,
	}
}

func (w *prefixedLineWriter) Write(p []byte) (int, error) {
	if w.prefix == "" {
		return w.w.Write(p)
	}

	written := 0
	remaining := p
	for len(remaining) > 0 {
		if w.atLineStart {
			if _, err := io.WriteString(w.w, w.prefix); err != nil {
				return written, err
			}
			w.atLineStart = false
		}

		newlineIdx := bytes.IndexByte(remaining, '\n')
		if newlineIdx == -1 {
			n, err := w.w.Write(remaining)
			written += n
			return written, err
		}

		chunk := remaining[:newlineIdx+1]
		n, err := w.w.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		w.atLineStart = true
		remaining = remaining[newlineIdx+1:]
	}

	return written, nil
}

type fileFollowTarget struct {
	Path   string
	Offset int64
	Render func(line []byte) error
	Finish func()
}

func followFiles(ctx context.Context, targets []fileFollowTarget) error {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		for _, target := range targets {
			if target.Finish != nil {
				target.Finish()
			}
		}
	}()

	for {
		for i := range targets {
			if err := pollFile(&targets[i]); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func pollFile(target *fileFollowTarget) error {
	f, err := os.Open(target.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < target.Offset {
		target.Offset = 0
	}
	if info.Size() == target.Offset {
		return nil
	}

	if _, err := f.Seek(target.Offset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			target.Offset += int64(len(line))
			if renderErr := target.Render(cloneBytes(line)); renderErr != nil {
				return renderErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func replayFileLines(path string, lines int, render func(line []byte) error) error {
	if lines == 0 {
		return streamAllLines(path, render)
	}

	buffered, err := readTailLines(path, lines)
	if err != nil {
		return err
	}
	for _, line := range buffered {
		if err := render(line); err != nil {
			return err
		}
	}
	return nil
}

func streamAllLines(path string, render func(line []byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if renderErr := render(cloneBytes(line)); renderErr != nil {
				return renderErr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func readTailLines(path string, limit int) ([][]byte, error) {
	if limit <= 0 {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	lines := make([][]byte, 0, limit)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lines = append(lines, cloneBytes(line))
			if len(lines) > limit {
				lines = lines[1:]
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return lines, nil
		}
		return nil, err
	}
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func cloneBytes(in []byte) []byte {
	out := make([]byte, len(in))
	copy(out, in)
	return out
}
