package eventq

import "context"

// Offer performs a non-blocking send.
// It returns true when the value was sent and false when the channel is full.
func Offer[T any](ch chan<- T, value T) (sent bool) {
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()
	select {
	case ch <- value:
		return true
	default:
		return false
	}
}

// OfferContext performs a non-blocking send that also respects context cancellation.
// It returns false if ctx is already done or if the channel is full.
func OfferContext[T any](ctx context.Context, ch chan<- T, value T) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}
	return Offer(ch, value)
}
