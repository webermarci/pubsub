// Package pubsub provides typed, in-memory topics for one-to-many
// communication.
//
// # Getting started
//
// Create a typed Topic, subscribe with a context, and publish values to every
// active subscriber:
//
//	topic := pubsub.New[OrderCreated]()
//	events := topic.Subscribe(ctx)
//
//	go func() {
//		for event := range events {
//			handle(event)
//		}
//	}()
//
//	if err := topic.Publish(ctx, OrderCreated{ID: "order_123"}); err != nil {
//		// handle cancellation or an unavailable subscriber
//	}
//
// A Topic is identified by the value that holds it. It does not use string or
// generic keys to route publications and does not need to be started or
// supervised.
//
// # Semantics
//
// Subscriptions are unbuffered by default. Publish blocks until every
// subscription that was active when publishing began receives the value, so a
// subscriber that is not receiving applies direct backpressure. Subscribe
// accepts WithBuffer to opt into a bounded per-subscription buffer. A buffered
// subscription applies backpressure again when its buffer is full. Publish
// accepts a context to stop waiting. Delivery that already reached a
// subscriber cannot be rolled back. Publishing to a topic with no subscribers
// returns immediately.
//
// Subscribe returns a receive-only channel. Canceling its context removes the
// subscription and closes the channel. If buffered values remain, they can be
// drained before the closed channel is observed. Subscriptions do not have a
// separate Close method.
//
// A handler that performs slow work before receiving the next value also
// applies backpressure. When asynchronous processing is required, consume the
// channel in a goroutine and compose it with an application-owned worker or
// queue. The package does not silently buffer, drop, retry, or replay values.
package pubsub
