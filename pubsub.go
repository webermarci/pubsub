// Package pubsub implements a lightweight, generic, in-memory publisher/subscriber system.
// It supports topic-based broadcasting and handles subscriber lifecycle via context.Context.
package pubsub

import (
	"context"
	"sync"
)

// PubSub is a thread-safe, generic topic manager.
// It allows multiple subscribers to listen to specific topics and
// publishers to broadcast messages to those topics.
//
// T represents the type of the payload being published.
type PubSub[T any] struct {
	capacity    int
	subscribers map[string][]chan T
	mutex       sync.RWMutex
}

// New creates a new PubSub instance with a default channel capacity of 1.
func New[T any]() *PubSub[T] {
	return &PubSub[T]{
		capacity:    1,
		subscribers: make(map[string][]chan T),
	}
}

// WithCapacity sets the buffer size for new subscription channels.
func (pubsub *PubSub[T]) WithCapacity(capacity int) *PubSub[T] {
	pubsub.capacity = capacity
	return pubsub
}

// Publish broadcasts the payload to all subscribers of the given topic.
//
// This method is non-blocking. If a subscriber's channel is full (slow consumer),
// the message is dropped for that specific subscriber to prevent blocking
// the publisher or other subscribers.
func (pubsub *PubSub[T]) Publish(topic string, payload T) {
	pubsub.mutex.RLock()
	defer pubsub.mutex.RUnlock()

	for _, ch := range pubsub.subscribers[topic] {
		select {
		case ch <- payload:
		default:
		}
	}
}

// Subscribe registers a new channel for the given topic.
// It returns a read-only channel that receives published messages.
//
// The subscription is tied to the provided context. When ctx is canceled
// or times out, the subscription is automatically removed, and the
// returned channel is closed.
//
// Panic: This function panics if a nil context is provided.
func (pubsub *PubSub[T]) Subscribe(ctx context.Context, topic string) <-chan T {
	if ctx == nil {
		panic("pubsub: nil context provided to Subscribe")
	}

	channel := make(chan T, pubsub.capacity)

	pubsub.mutex.Lock()
	pubsub.subscribers[topic] = append(pubsub.subscribers[topic], channel)
	pubsub.mutex.Unlock()

	go func() {
		<-ctx.Done()
		pubsub.mutex.Lock()
		defer pubsub.mutex.Unlock()

		subscribers := pubsub.subscribers[topic]
		for i, ch := range subscribers {
			if ch == channel {
				copy(subscribers[i:], subscribers[i+1:])
				subscribers[len(subscribers)-1] = nil
				pubsub.subscribers[topic] = subscribers[:len(subscribers)-1]
				break
			}
		}

		if len(pubsub.subscribers[topic]) == 0 {
			delete(pubsub.subscribers, topic)
		}

		close(channel)
	}()

	return channel
}
