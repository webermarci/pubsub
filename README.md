# PubSub

[![Go Reference](https://pkg.go.dev/badge/github.com/webermarci/pubsub.svg)](https://pkg.go.dev/github.com/webermarci/pubsub)
[![Test](https://github.com/webermarci/pubsub/actions/workflows/test.yml/badge.svg)](https://github.com/webermarci/pubsub/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A small, typed, in-memory topic for one-to-many communication in Go.

## Design

- `Topic[T]` is identified by the topic value itself; there are no string or generic keys.
- Subscriptions use unbuffered, receive-only channels by default; `WithBuffer` enables bounded buffering for an individual subscription.
- `Publish` blocks until every matching active subscriber receives the value.
- A publish context can stop waiting and returns its cancellation error.
- Subscription lifetime is controlled by its context; canceling it closes the channel.
- `SubscribeAs[S]` filters heterogeneous topics using Go's normal assignability rules; a nonmatching subscription does not apply backpressure.
- The package does not silently buffer, drop, retry, replay, or process messages in internal goroutines.

If a caller wants asynchronous behavior, it explicitly starts a goroutine or adds an application-owned worker or queue.

## Quick start

```go
package main

import (
	"context"
	"fmt"

	"github.com/webermarci/pubsub"
)

type OrderCreated struct {
	ID string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orders := pubsub.New[OrderCreated]()
	events := orders.Subscribe(ctx)

	go func() {
		for event := range events {
			fmt.Printf("new order: %s\n", event.ID)
		}
	}()

	if err := orders.Publish(ctx, OrderCreated{ID: "ORD-12345"}); err != nil {
		panic(err)
	}
}
```

Every subscriber receives every publication. By default, a slow subscriber applies backpressure to the publisher and to the other subscribers of that topic. An explicit buffer can absorb a bounded burst, but backpressure resumes when it fills. Use a goroutine and an application-owned queue when that is not desired.

## Heterogeneous topics

Use `SubscribeAs[S]` when a topic carries an interface and a subscriber only
needs values assignable to a particular concrete or interface type:

```go
type Event interface {
	Event()
}

type Started struct{}
func (Started) Event() {}

type Failure interface {
	Event
	Error() string
}

events := pubsub.New[Event]()

all := events.Subscribe(ctx)
started := events.SubscribeAs[Started](ctx)
failures := events.SubscribeAs[Failure](ctx)
```

`all` receives every published event. `started` receives `Started` values, and
`failures` receives every value implementing `Failure`. Matching uses ordinary
Go type assertion and assignability semantics. A nonmatching subscription is
skipped immediately and does not participate in backpressure for that
publication.
