# PubSub

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/pubsub.svg)](https://pkg.go.dev/github.com/webermarci/pubsub)
[![Test](https://github.com/webermarci/pubsub/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/pubsub/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A lightweight, generic, in-memory publisher/subscriber library for Go.

This package provides a thread-safe, type-safe message bus that leverages Go
1.18+ generics. It is designed for high-throughput scenarios where non-blocking
publishing is preferred over guaranteed delivery.

## Features

- **Generic key & payload types:** Strongly typed as `PubSub[K comparable, V any]` so topic keys must be comparable and payloads are type-checked at compile time.
- **Per-subscriber buffered channels:** Each subscription receives its own buffered channel with the capacity you pass to `New`. This isolates slow consumers from fast producers.
- **Context-aware subscriptions:** Subscriptions are tied to a `context.Context`. Canceling the context (or letting it expire) removes the subscription and closes the subscriber's channel automatically.
- **Non-blocking publish (drop-on-full):** `Publish` attempts to deliver the message to every subscriber without blocking; if a subscriber's channel is full the message is dropped for that subscriber only, preventing a slow consumer from blocking publishers or other subscribers.
- **On-demand topic lifecycle:** Topics are created when the first subscriber subscribes and removed when the last subscriber unsubscribes, keeping the internal map small.
- **Concurrency-safe & low-overhead:** Built with `sync.Map` and per-topic `sync.RWMutex` to allow concurrent publishers and subscribers. The implementation is optimized for high-throughput scenarios with minimal allocations in the hot path.
- **Graceful and idempotent close:** Calling `Close()` stops accepting new activity, closes all active subscriber channels, and is safe to call multiple times. After closing, `Publish` becomes a no-op and new `Subscribe` calls return a closed channel.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/webermarci/pubsub"
)

func main() {
	// Create a new PubSub instance for string keys and string messages.
	// Set a buffer capacity of 10 messages per subscriber.
	ps := pubsub.New[string, string](10)

	ctx, cancel := context.WithCancel(context.Background())

	// The returned channel will close automatically when ctx is done.
	channel := ps.Subscribe(ctx, "user:updates")

	go func() {
		for message := range channel {
			fmt.Printf("Received: %s\n", message)
		}
		fmt.Println("Subscriber stopped")
	}()

	ps.Publish("user:updates", "user_connected")
	ps.Publish("user:updates", "user_disconnected")

	// Canceling the context removes the subscription from the internal map
	cancel()
}
```

## Benchmark

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/pubsub
cpu: Apple M5
BenchmarkPublish_SingleSubscriber-10   51463308    23.03 ns/op      0 B/op    0 allocs/op
BenchmarkPublish_RunParallel-10        13082737    91.19 ns/op      0 B/op    0 allocs/op
BenchmarkPublish_FanOut100-10             90074    13288 ns/op      0 B/op    0 allocs/op
BenchmarkPublish_Contention-10           623960     1930 ns/op    384 B/op    1 allocs/op
```
