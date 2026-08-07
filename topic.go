package pubsub

import (
	"context"
	"sync"
)

// Topic broadcasts each published value to every active subscription.
//
// Topic is passive and does not need to be started or supervised. Its
// subscriptions are unbuffered by default, so Publish applies direct
// backpressure until every active subscriber receives the value.
type Topic[T any] struct {
	subscribers   map[*subscription[T]]struct{}
	subscribersMu sync.RWMutex
}

// New creates a typed, unbuffered topic.
func New[T any]() *Topic[T] {
	return &Topic[T]{
		subscribers: make(map[*subscription[T]]struct{}),
	}
}

// Subscribe creates an independent subscription whose lifetime is bound to
// ctx. The returned channel closes when ctx is canceled. Subscriptions are
// unbuffered by default; use WithBuffer to enable bounded buffering.
func (t *Topic[T]) Subscribe(ctx context.Context, opts ...SubscriptionOption) <-chan T {
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
		return closedChannel[T]()
	}

	subscription := &subscription[T]{
		topic:  t,
		values: make(chan T, config.buffer),
		done:   make(chan struct{}),
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

// Publish sends value to every subscription that is active when publishing
// begins. A canceled context stops delivery and returns its error. Delivery to
// subscriptions that already received the value cannot be rolled back.
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
	subscribers := make([]*subscription[T], 0, len(t.subscribers))
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
