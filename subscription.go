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

type subscriber[T any] interface {
	publish(context.Context, T) error
}

func subscribe[T, S any](
	t *Topic[T],
	ctx context.Context,
	match func(T) (S, bool),
	opts ...SubscriptionOption,
) <-chan S {
	if t == nil {
		panic("pubsub: cannot subscribe to a nil topic")
	}
	if ctx == nil {
		panic("pubsub: subscription context cannot be nil")
	}

	config := subscriptionConfig{}
	for _, opt := range opts {
		if opt == nil {
			panic("pubsub: subscription option cannot be nil")
		}
		opt(&config)
	}

	if ctx.Err() != nil {
		return closedChannel[S]()
	}

	subscription := &subscription[T, S]{
		topic:  t,
		values: make(chan S, config.buffer),
		done:   make(chan struct{}),
		match:  match,
	}

	t.subscribersMu.Lock()
	if ctx.Err() != nil {
		t.subscribersMu.Unlock()
		close(subscription.values)
		return subscription.values
	}
	t.subscribers[subscription] = struct{}{}
	t.subscribersMu.Unlock()

	context.AfterFunc(ctx, subscription.close)
	return subscription.values
}

func (t *Topic[T]) remove(subscription subscriber[T]) {
	t.subscribersMu.Lock()
	delete(t.subscribers, subscription)
	t.subscribersMu.Unlock()
}

type subscription[T, S any] struct {
	topic  *Topic[T]
	values chan S
	done   chan struct{}
	match  func(T) (S, bool)
	closed atomic.Bool
	mu     sync.Mutex
}

func (s *subscription[T, S]) close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}

	close(s.done)
	s.topic.remove(s)

	s.mu.Lock()
	close(s.values)
	s.mu.Unlock()
}

func (s *subscription[T, S]) publish(ctx context.Context, value T) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	matched, ok := s.match(value)
	if !ok {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return nil
	}

	select {
	case s.values <- matched:
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
