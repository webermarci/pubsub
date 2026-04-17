# PubSub

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/pubsub.svg)](https://pkg.go.dev/github.com/webermarci/pubsub)
[![Test](https://github.com/webermarci/pubsub/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/pubsub/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A lightweight, generic, in-memory publisher/subscriber library for Go.

This package provides a thread-safe, type-safe message bus designed for high-throughput scenarios where non-blocking publishing and deep observability are required.

## Features

- **Generic & Type-Safe:** Fully leverages Go generics. Payloads are type-checked at compile time, and topic keys can be any `comparable` type.
- **Observer Interface:** Built-in hooks to monitor the system's "weather"—track publishing, subscriber counts, message drops, and system shutdown.
- **Non-blocking Publish (Drop-on-Full):** Uses a `select-default` pattern to ensure slow consumers never block the producer or other subscribers.
- **Zero-Allocation Hot Path:** Optimized to ensure that `Publish` calls involve zero heap allocations, even with active observers.
- **Context-Aware Lifecycle:** Subscriptions are tied to a `context.Context`. Canceling the context automatically cleans up the subscription and closes the subscriber's channel.
- **Efficient Memory Management:** Topics are created/deleted on-demand. Internal subscriber slices automatically shrink when they become sparse to prevent memory leaks in long-running processes.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	
	"github.com/webermarci/pubsub"
)

func main() {
	ps := pubsub.New[string, string](10)
	defer ps.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	messages := ps.Subscribe(ctx, "orders")

	go func() {
		for msg := range messages {
			fmt.Printf("New order: %s\n", msg)
		}
	}()

	ps.Publish("orders", "ORD-12345")
	
	ps.Close()
	cancel()
}
```

## Benchmark

```bash
goos: darwin
goarch: arm64
pkg: github.com/webermarci/pubsub
cpu: Apple M5
BenchmarkPublish_NoObserver-10     49885567      23.7 ns/op     0 B/op   0 allocs/op
BenchmarkPublish_WithObserver-10   34035633      36.0 ns/op     0 B/op   0 allocs/op
BenchmarkPublish_Contention-10       796538    1692.3 ns/op   431 B/op   2 allocs/op
BenchmarkPublish_FanOut100-10         89859   13322.1 ns/op     0 B/op   0 allocs/op
```
