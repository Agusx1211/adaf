package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

const maxLineSize = 1024 * 1024 // 1 MB

// Parse reads NDJSON lines from r and sends parsed events on the returned
// channel. The channel is closed when the reader reaches EOF or the context
// is cancelled.
func Parse(ctx context.Context, r io.Reader) <-chan RawEvent {
	ch := make(chan RawEvent, 64)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, maxLineSize), maxLineSize)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			raw := make([]byte, len(line))
			copy(raw, line)

			var ev ClaudeEvent
			if err := json.Unmarshal(raw, &ev); err != nil {
				ch <- RawEvent{Raw: raw, Err: err}
				continue
			}

			ch <- RawEvent{Raw: raw, Parsed: ev}
		}

		if err := scanner.Err(); err != nil {
			ch <- RawEvent{Err: err}
		}
	}()
	return ch
}
