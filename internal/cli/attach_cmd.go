package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/agusx1211/adaf/internal/events"
	"github.com/agusx1211/adaf/internal/session"
	"github.com/agusx1211/adaf/internal/stream"
)

var attachCmd = &cobra.Command{
	Use:     "attach [loop-name|session-id]",
	Aliases: []string{"reattach", "connect"},
	Short:   "Attach to a running loop or session",
	Long: `Attach to a running adaf loop or session and stream daemon events to stdout.

With no arguments, attaches to the only running session (if exactly one exists).
With a loop name, attaches to the running session for that loop.
With a numeric session ID, attaches to that specific session.

Use --json to output NDJSON event envelopes.
Use 'adaf sessions' to list available sessions.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAttach,
}

func init() {
	attachCmd.Flags().Bool("json", false, "Output events as NDJSON")
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	if session.IsAgentContext() {
		return fmt.Errorf("session management is not available inside an agent context")
	}

	// Find the session to attach to.
	var meta *session.SessionMeta
	var err error

	if len(args) == 0 {
		// No argument: auto-attach to the only running session.
		meta, err = session.FindOnlyRunningSession()
		if err != nil {
			return err
		}
	} else {
		// Try loop name first, then fall back to session ID / profile match.
		meta, err = session.FindRunningByLoopName(args[0])
		if err != nil {
			meta, err = session.FindSessionByPartial(args[0])
			if err != nil {
				return err
			}
		}
	}

	if !session.IsActiveStatus(meta.Status) {
		return fmt.Errorf("session %d is not running (status: %s)", meta.ID, meta.Status)
	}

	// Connect to the session daemon.
	client, err := session.ConnectToSession(meta.ID)
	if err != nil {
		return fmt.Errorf("connecting to session %d: %w", meta.ID, err)
	}
	defer client.Close()

	fmt.Printf("  %sAttaching to session #%d (%s)...%s\n", colorDim, meta.ID, meta.ProfileName, colorReset)

	jsonMode, _ := cmd.Flags().GetBool("json")
	eventCh := make(chan any, 256)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cancelFunc := func() {
		_ = client.Cancel()
		cancel()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n%sReceived interrupt, stopping agent...%s\n", colorDim, colorReset)
		cancelFunc()
	}()

	go func() {
		errCh <- client.StreamEvents(eventCh, nil)
	}()

	for ev := range eventCh {
		if ctx.Err() != nil {
			continue
		}
		if jsonMode {
			printEventJSON(os.Stdout, ev)
		} else {
			printEventText(os.Stdout, ev)
		}
	}

	if err := <-errCh; err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

func printEventJSON(w io.Writer, ev any) {
	typeName := fmt.Sprintf("%T", ev)
	if i := strings.LastIndex(typeName, "."); i >= 0 {
		typeName = typeName[i+1:]
	}
	wrapper := map[string]any{"type": typeName, "data": ev}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "%s\n", data)
}

func printEventText(w io.Writer, ev any) {
	switch e := ev.(type) {
	case events.AgentStartedMsg:
		fmt.Fprintf(w, "%s--- Agent started (session #%d) ---%s\n", styleBoldCyan, e.SessionID, colorReset)
	case events.AgentPromptMsg:
		prompt := e.Prompt
		if len(prompt) > 200 {
			prompt = prompt[:200] + "..."
		}
		fmt.Fprintf(w, "%s[prompt]%s %s\n", colorDim, colorReset, prompt)
	case events.AgentEventMsg:
		printStreamEvent(w, e.Event)
	case events.AgentRawOutputMsg:
		fmt.Fprint(w, e.Data)
	case events.AgentFinishedMsg:
		status := "ok"
		if e.Err != nil {
			status = e.Err.Error()
		}
		fmt.Fprintf(w, "\n%s--- Agent finished (session #%d, status: %s) ---%s\n", styleBoldCyan, e.SessionID, status, colorReset)
	case events.SpawnStatusMsg:
		for _, sp := range e.Spawns {
			fmt.Fprintf(w, "  %s[spawn #%d]%s %s (%s) — %s\n", colorBlue, sp.ID, colorReset, sp.Profile, sp.Role, sp.Status)
		}
	case events.LoopStepStartMsg:
		fmt.Fprintf(w, "%s=== Loop step %d/%d starting (cycle %d, %s) ===%s\n", styleBoldGreen, e.StepIndex+1, e.TotalSteps, e.Cycle, e.Profile, colorReset)
	case events.LoopStepEndMsg:
		fmt.Fprintf(w, "%s=== Loop step %d/%d ended ===%s\n", colorGreen, e.StepIndex+1, e.TotalSteps, colorReset)
	case events.LoopDoneMsg:
		fmt.Fprintf(w, "%s=== Loop done (%s) ===%s\n", styleBoldGreen, e.Reason, colorReset)
	case events.AgentLoopDoneMsg:
		status := "ok"
		if e.Err != nil {
			status = e.Err.Error()
		}
		fmt.Fprintf(w, "%s=== Agent loop done (%s) ===%s\n", styleBoldCyan, status, colorReset)
	case events.SessionSnapshotMsg:
		fmt.Fprintf(w, "%s[snapshot] Loop: cycle %d step %d/%d (%s)%s\n", colorDim, e.Loop.Cycle, e.Loop.StepIndex+1, e.Loop.TotalSteps, e.Loop.Profile, colorReset)
		if e.Session != nil {
			fmt.Fprintf(w, "%s[snapshot] Session: %s (%s) — %s%s\n", colorDim, e.Session.Agent, e.Session.Model, e.Session.Status, colorReset)
		}
	case events.GuardrailViolationMsg:
		fmt.Fprintf(w, "%s[guardrail]%s role=%s tool=%s\n", styleBoldYellow, colorReset, e.Role, e.Tool)
	}
}

func printStreamEvent(w io.Writer, ev stream.ClaudeEvent) {
	switch ev.Type {
	case "content_block_delta":
		if ev.Delta != nil {
			if ev.Delta.Text != "" {
				fmt.Fprint(w, ev.Delta.Text)
			}
		}
	case "content_block_start":
		if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
			fmt.Fprintf(w, "\n%s[tool: %s]%s ", colorYellow, ev.ContentBlock.Name, colorReset)
		}
	case "message_stop":
		fmt.Fprintln(w)
	}
}
