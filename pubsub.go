// Package pubsub implements a lightweight, generic, in-memory publisher/subscriber system.
// It supports topic-based broadcasting and handles subscriber lifecycle via context.Context.
package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
)

type sub[V any] struct {
	ch   chan V
	stop func() bool
}

type topicState[V any] struct {
	mu      sync.RWMutex
	subs    []sub[V]
	deleted bool
}

// PubSub is a thread-safe, generic topic manager.
// It allows multiple subscribers to listen to specific topics and
// publishers to broadcast messages to those topics.
//
// T represents the type of the payload being published.
type PubSub[K comparable, V any] struct {
	capacity   int
	topics     sync.Map
	closed     atomic.Bool
	closedChan chan V
}

// New creates a new PubSub instance.
func New[K comparable, V any](capacity int) *PubSub[K, V] {
	cc := make(chan V)
	close(cc)

	return &PubSub[K, V]{
		capacity:   capacity,
		closedChan: cc,
	}
}

// Publish broadcasts the payload to all subscribers of the given topic.
//
// This method is non-blocking. If a subscriber's channel is full (slow consumer),
// the message is dropped for that specific subscriber to prevent blocking
// the publisher or other subscribers.
func (p *PubSub[K, V]) Publish(topic K, payload V) {
	if p.closed.Load() {
		return
	}

	val, ok := p.topics.Load(topic)
	if !ok {
		return
	}
	state := val.(*topicState[V])

	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, s := range state.subs {
		select {
		case s.ch <- payload:
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
func (p *PubSub[K, V]) Subscribe(ctx context.Context, topic K) <-chan V {
	if ctx == nil || ctx.Err() != nil || p.closed.Load() {
		return p.closedChan
	}

	ch := make(chan V, p.capacity)
	var state *topicState[V]

	for {
		if p.closed.Load() {
			return p.closedChan
		}

		val, ok := p.topics.Load(topic)
		if !ok {
			val, _ = p.topics.LoadOrStore(topic, &topicState[V]{})
		}
		state = val.(*topicState[V])

		state.mu.Lock()
		if p.closed.Load() {
			state.mu.Unlock()
			return p.closedChan
		}

		if state.deleted {
			state.mu.Unlock()
			continue
		}

		stop := context.AfterFunc(ctx, func() {
			p.removeSubscriber(topic, ch)
		})

		state.subs = append(state.subs, sub[V]{
			ch:   ch,
			stop: stop,
		})

		state.mu.Unlock()
		break
	}

	return ch
}

func (p *PubSub[K, V]) removeSubscriber(topic K, ch chan V) {
	val, ok := p.topics.Load(topic)
	if !ok {
		return
	}
	state := val.(*topicState[V])

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.deleted {
		return
	}

	found := false
	for i, s := range state.subs {
		if s.ch == ch {
			found = true
			lastIdx := len(state.subs) - 1
			state.subs[i] = state.subs[lastIdx]
			state.subs[lastIdx] = sub[V]{}
			state.subs = state.subs[:lastIdx]
			break
		}
	}

	if !found {
		return
	}

	if n := len(state.subs); n > 0 && n <= cap(state.subs)/4 {
		newSubs := make([]sub[V], n)
		copy(newSubs, state.subs)
		state.subs = newSubs
	}

	if len(state.subs) == 0 {
		state.deleted = true
		p.topics.Delete(topic)
	}

	close(ch)
}

// Close gracefully shuts down the PubSub instance.
// It prevents any new publishers or subscribers and safely closes all
// currently active subscriber channels. Safe to call multiple times.
func (p *PubSub[K, V]) Close() {
	if p.closed.CompareAndSwap(false, true) {
		p.topics.Range(func(key, value any) bool {
			state := value.(*topicState[V])

			state.mu.Lock()
			if !state.deleted {
				for _, s := range state.subs {
					s.stop()
					close(s.ch)
				}
				state.subs = nil
				state.deleted = true
				p.topics.Delete(key)
			}
			state.mu.Unlock()

			return true
		})
	}
}
