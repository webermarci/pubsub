package pubsub

import (
	"context"
	"sync"
)

type PubSub[T any] struct {
	capacity    int
	subscribers map[string][]chan T
	mutex       sync.RWMutex
}

func New[T any]() *PubSub[T] {
	return &PubSub[T]{
		capacity:    1,
		subscribers: make(map[string][]chan T),
	}
}

func (pubsub *PubSub[T]) WithCapacity(capacity int) *PubSub[T] {
	pubsub.capacity = capacity
	return pubsub
}

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
