package pubsub

import (
	"context"
	"sync"
)

// Topic broadcasts each published value to every matching active subscription.
//
// Topic is passive and does not need to be started or supervised. Its
// subscriptions are unbuffered by default, so Publish applies direct
// backpressure until every matching active subscriber receives the value.
type Topic[T any] struct {
	subscribers   map[subscriber[T]]struct{}
	subscribersMu sync.RWMutex
}

// New creates a typed, unbuffered topic.
func New[T any]() *Topic[T] {
	return &Topic[T]{
		subscribers: make(map[subscriber[T]]struct{}),
	}
}

// Subscribe creates an independent subscription whose lifetime is bound to
// ctx. The returned channel closes when ctx is canceled. Subscriptions are
// unbuffered by default; use WithBuffer to enable bounded buffering.
func (t *Topic[T]) Subscribe(ctx context.Context, opts ...SubscriptionOption) <-chan T {
	return subscribe(t, ctx, func(value T) (T, bool) {
		return value, true
	}, opts...)
}

// SubscribeAs creates an independent subscription that receives values
// published on t that are assignable to S. Values that are not assignable to S
// do not participate in backpressure for this subscription.
//
// The subscription lifetime and buffering behavior are the same as Subscribe.
func (t *Topic[T]) SubscribeAs[S any](ctx context.Context, opts ...SubscriptionOption) <-chan S {
	return subscribe(t, ctx, func(value T) (S, bool) {
		matched, ok := any(value).(S)
		return matched, ok
	}, opts...)
}

// Publish sends value to every matching subscription that is active when
// publishing begins. A canceled context stops delivery and returns its error.
// Delivery to subscriptions that already received the value cannot be rolled
// back.
func (t *Topic[T]) Publish(ctx context.Context, value T) error {
	if t == nil {
		panic("pubsub: cannot publish to a nil topic")
	}
	if ctx == nil {
		panic("pubsub: publish context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	t.subscribersMu.RLock()
	subscribers := make([]subscriber[T], 0, len(t.subscribers))
	for subscriber := range t.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	t.subscribersMu.RUnlock()

	for _, subscriber := range subscribers {
		if err := subscriber.publish(ctx, value); err != nil {
			return err
		}
	}

	return nil
}

// SubscriberCount returns the number of currently active subscriptions.
func (t *Topic[T]) SubscriberCount() int {
	if t == nil {
		return 0
	}

	t.subscribersMu.RLock()
	count := len(t.subscribers)
	t.subscribersMu.RUnlock()
	return count
}
