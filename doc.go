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
// A topic whose published type is an interface can also expose filtered,
// strongly typed subscriptions:
//
//	events := pubsub.New[Event]()
//	all := events.Subscribe(ctx)
//	started := events.SubscribeAs[Started](ctx)
//	failures := events.SubscribeAs[Failure](ctx)
//
// SubscribeAs[S] receives values assignable to S using ordinary Go type
// assertion semantics. A subscription that does not match a publication is
// skipped and does not participate in backpressure for that publication.
//
// A Topic is identified by the value that holds it. It does not use string or
// generic keys to route publications and does not need to be started or
// supervised.
//
// # Semantics
//
// Subscriptions are unbuffered by default. Publish blocks until every matching
// subscription that was active when publishing began receives the value, so a
// matching subscriber that is not receiving applies direct backpressure.
// Subscribe and SubscribeAs accept WithBuffer to opt into a bounded
// per-subscription buffer. A buffered subscription applies backpressure again
// when its buffer is full. Publish accepts a context to stop waiting. Delivery
// that already reached a subscriber cannot be rolled back. Publishing to a
// topic with no matching subscribers returns immediately.
//
// Subscribe and SubscribeAs return receive-only channels. Canceling a
// subscription's context removes it and closes its channel. If buffered values
// remain, they can be drained before the closed channel is observed.
// Subscriptions do not have a separate Close method.
//
// A handler that performs slow work before receiving the next value also
// applies backpressure. When asynchronous processing is required, consume the
// channel in a goroutine and compose it with an application-owned worker or
// queue. The package does not silently buffer, drop, retry, or replay values.
package pubsub
