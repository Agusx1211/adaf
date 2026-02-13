package stream

import (
	"context"

	"github.com/agusx1211/adaf/internal/debug"
	"github.com/agusx1211/adaf/internal/eventq"
)

func offerRawEvent(ctx context.Context, ch chan<- RawEvent, ev RawEvent, parser string) bool {
	if eventq.OfferContext(ctx, ch, ev) {
		return true
	}
	debug.LogKV("stream", "dropping parsed event due to backpressure", "parser", parser, "event_type", ev.Parsed.Type, "has_err", ev.Err != nil)
	return false
}
