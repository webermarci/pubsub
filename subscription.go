package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
)

type subscriptionConfig struct {
	buffer int
}

// SubscriptionOption configures one subscription.
type SubscriptionOption func(*subscriptionConfig)

// WithBuffer enables a bounded buffer for one subscription.
//
// The default buffer size is zero. When the buffer is full, Publish applies
// backpressure until the subscriber receives another value.
func WithBuffer(size int) SubscriptionOption {
	if size < 0 {
		panic("pubsub: subscription buffer size cannot be negative")
	}

	return func(config *subscriptionConfig) {
		config.buffer = size
	}
}

func (t *Topic[T]) remove(subscription *subscription[T]) {
	t.subscribersMu.Lock()
	delete(t.subscribers, subscription)
	t.subscribersMu.Unlock()
}

type subscription[T any] struct {
	topic  *Topic[T]
	values chan T
	done   chan struct{}
	closed atomic.Bool
	mu     sync.Mutex
}

func (s *subscription[T]) close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}

	close(s.done)
	s.topic.remove(s)

	s.mu.Lock()
	close(s.values)
	s.mu.Unlock()
}

func (s *subscription[T]) publish(ctx context.Context, value T) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return nil
	}

	select {
	case s.values <- value:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return nil
	}
}

func closedChannel[T any]() <-chan T {
	channel := make(chan T)
	close(channel)
	return channel
}
