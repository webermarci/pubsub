# PubSub

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/pubsub.svg)](https://pkg.go.dev/github.com/webermarci/pubsub)
[![Test](https://github.com/webermarci/pubsub/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/pubsub/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A lightweight, generic, in-memory publisher/subscriber library for Go.

This package provides a thread-safe, type-safe message bus that leverages Go
1.18+ generics. It is designed for high-throughput scenarios where non-blocking
publishing is preferred over guaranteed delivery.

## Features

- **Generic Type Support:** strictly typed payloads using `[T any]`.
- **Context-Aware:** Subscriptions are tied to a `context.Context`. Canceling
  the context automatically unsubscribes the listener and cleans up resources.
- **Non-Blocking Publishers:** Uses the "drop on full" strategy. If a subscriber
  is too slow (channel full), the message is skipped for that specific
  subscriber to prevent blocking the publisher.
- **Concurrency Safe:** Fully thread-safe using `sync.RWMutex`.

## Installation

```bash
go get github.com/webermarci/pubsub
```

## Usage

### Basic Example

```go
package main

import (
	"context"
	"fmt"
	"time"
	
	"github.com/webermarci/pubsub"
)

func main() {
	// 1. Create a new PubSub instance for string messages.
	// Set a buffer capacity of 10 messages per subscriber.
	ps := pubsub.New[string]().WithCapacity(10)

	// 2. Create a context for the subscriber
	ctx, cancel := context.WithCancel(context.Background())

	// 3. Subscribe to a topic
	// The returned channel will close automatically when ctx is done.
	channel := ps.Subscribe(ctx, "user:updates")

	// 4. Consume messages in a goroutine
	go func() {
		for message := range channel {
			fmt.Printf("Received: %s\n", message)
		}
		fmt.Println("Subscriber stopped")
	}()

	// 5. Publish messages
	ps.Publish("user:updates", "user_connected")
	ps.Publish("user:updates", "user_disconnected")

	// 6. Cleanup
	// Canceling the context removes the subscription from the internal map
	cancel()
}
```

## Design Principles

### Lifecycle Management

Unlike many PubSub implementations that require a manual `Unsubscribe()` method,
this package relies entirely on `context.Context`:

- Pass a cancellable context (or one with a timeout) to `Subscribe`

- When the context is canceled, the package automatically removes the subscriber
  from the internal map and closes the consumption channel

- This architecture prevents memory leaks in long-running applications (like
  WebSocket servers) where clients disconnect frequently

### Slow Consumers

To ensure high throughput and prevent a single slow consumer from deadlocking
the entire system, `Publish` is strictly non-blocking:

- If a subscriber's channel buffer is full, the message is dropped for that
  specific subscriber

- Publishers never wait for consumers

- Use `.WithCapacity(n)` to increase the buffer size if you anticipate bursty
  traffic

### Performance

The following benchmarks were run on an Apple M1 (ARM64) using Go 1.23.

The results demonstrate extremely low overhead for single subscribers (~22ns)
and efficient scaling for fan-out scenarios.

| Benchmark                         | Iterations | Time/Op  | Alloc/Op | Description                       |
| --------------------------------- | ---------- | -------- | -------- | --------------------------------- |
| BenchmarkPublish_SingleSubscriber | 53,879,792 | 21.70 ns | 0 B      | 1 Publisher, 1 Subscriber         |
| BenchmarkPublish_FanOut100        | 197,169    | 6366 ns  | 0 B      | 1 Publisher, 100 Subscribers      |
| BenchmarkPublish_Contention       | 664,107    | 2098 ns  | 458 B    | Parallel Publishers, 1 Subscriber |
