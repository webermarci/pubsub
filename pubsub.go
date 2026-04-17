// Package pubsub implements a lightweight, generic, in-memory publisher/subscriber system.
// It supports topic-based broadcasting and handles subscriber lifecycle via context.Context.
package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
)

// Observer defines an interface for monitoring PubSub events such as publishing, dropping, subscribing, and unsubscribing.
type Observer[K comparable, V any] interface {
	OnPublish(topic K, payload V)
	OnDropped(topic K, payload V)
	OnSubscribed(topic K)
	OnUnsubscribed(topic K)
	OnClosed()
}

type sub[V any] struct {
	ch   chan V
	stop func() bool
}

type topicState[V any] struct {
	mu      sync.RWMutex
	subs    []sub[V]
	deleted bool
}

// PubSubOption defines a functional option for configuring the PubSub instance.
type PubSubOption[K comparable, V any] func(*PubSub[K, V])

// WithObserver allows you to set a custom Observer for monitoring PubSub events.
func WithObserver[K comparable, V any](observer Observer[K, V]) PubSubOption[K, V] {
	return func(p *PubSub[K, V]) {
		p.observer = observer
	}
}

// PubSub is a thread-safe, generic topic manager.
type PubSub[K comparable, V any] struct {
	capacity   int
	observer   Observer[K, V]
	topics     sync.Map
	closed     atomic.Bool
	closedChan chan V
}

// New creates a new PubSub instance.
func New[K comparable, V any](capacity int, opts ...PubSubOption[K, V]) *PubSub[K, V] {
	cc := make(chan V)
	close(cc)

	ps := &PubSub[K, V]{
		capacity:   capacity,
		closedChan: cc,
	}

	for _, opt := range opts {
		opt(ps)
	}

	return ps
}

// Publish broadcasts the payload to all subscribers of the given topic.
// It holds a read lock to ensure thread safety during broadcasting.
func (p *PubSub[K, V]) Publish(topic K, payload V) {
	if p.closed.Load() {
		return
	}

	obs := p.observer

	if obs != nil {
		obs.OnPublish(topic, payload)
	}

	val, ok := p.topics.Load(topic)
	if !ok {
		return
	}
	state := val.(*topicState[V])

	state.mu.RLock()
	defer state.mu.RUnlock()

	if state.deleted {
		return
	}

	for _, s := range state.subs {
		select {
		case s.ch <- payload:
		default:
			if obs != nil {
				obs.OnDropped(topic, payload)
			}
		}
	}
}

// Subscribe registers a new channel for the given topic.
func (p *PubSub[K, V]) Subscribe(ctx context.Context, topic K) <-chan V {
	if ctx == nil || ctx.Err() != nil || p.closed.Load() {
		return p.closedChan
	}

	ch := make(chan V, p.capacity)

	for {
		if p.closed.Load() {
			return p.closedChan
		}

		val, ok := p.topics.Load(topic)
		if !ok {
			val, _ = p.topics.LoadOrStore(topic, &topicState[V]{})
		}
		state := val.(*topicState[V])

		state.mu.Lock()

		if p.closed.Load() {
			state.mu.Unlock()
			return p.closedChan
		}

		if state.deleted {
			state.mu.Unlock()
			continue
		}

		if err := ctx.Err(); err != nil {
			state.mu.Unlock()
			return p.closedChan
		}

		stop := context.AfterFunc(ctx, func() {
			p.removeSubscriber(topic, ch)
		})

		state.subs = append(state.subs, sub[V]{
			ch:   ch,
			stop: stop,
		})

		if p.observer != nil {
			p.observer.OnSubscribed(topic)
		}

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

	foundIdx := -1
	for i, s := range state.subs {
		if s.ch == ch {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return
	}

	close(ch)

	lastIdx := len(state.subs) - 1
	state.subs[foundIdx] = state.subs[lastIdx]
	state.subs[lastIdx] = sub[V]{}
	state.subs = state.subs[:lastIdx]

	if n := len(state.subs); n > 0 && n <= cap(state.subs)/4 {
		newSubs := make([]sub[V], n)
		copy(newSubs, state.subs)
		state.subs = newSubs
	}

	if p.observer != nil {
		p.observer.OnUnsubscribed(topic)
	}

	if len(state.subs) == 0 {
		state.deleted = true
		p.topics.Delete(topic)
	}
}

// Close gracefully shuts down the PubSub instance and all active topics.
func (p *PubSub[K, V]) Close() {
	if p.closed.CompareAndSwap(false, true) {
		if p.observer != nil {
			p.observer.OnClosed()
		}

		p.topics.Range(func(key, value any) bool {
			state := value.(*topicState[V])

			state.mu.Lock()
			if !state.deleted {
				for _, s := range state.subs {
					if s.stop() {
						close(s.ch)
					}
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
