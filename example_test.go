package pubsub_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/webermarci/pubsub"
)

type exampleOrderCreated struct {
	ID string
}

func ExampleNew() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orders := pubsub.New[exampleOrderCreated]()
	created := orders.Subscribe(ctx, pubsub.WithBuffer(1))

	if err := orders.Publish(ctx, exampleOrderCreated{ID: "order-42"}); err != nil {
		panic(err)
	}
	fmt.Println((<-created).ID)

	// Output:
	// order-42
}

type exampleEvent interface {
	event()
}

type exampleStarted struct{}

func (exampleStarted) event() {}

type exampleFailure struct {
	err error
}

func (exampleFailure) event()          {}
func (f exampleFailure) Error() string { return f.err.Error() }

func ExampleTopic_SubscribeAs() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := pubsub.New[exampleEvent]()

	all := events.Subscribe(ctx, pubsub.WithBuffer(2))
	started := events.SubscribeAs[exampleStarted](ctx, pubsub.WithBuffer(1))
	failures := events.SubscribeAs[error](ctx, pubsub.WithBuffer(1))

	_ = events.Publish(ctx, exampleStarted{})
	_ = events.Publish(ctx, exampleFailure{err: errors.New("worker unavailable")})

	fmt.Printf("%T\n", <-all)
	fmt.Printf("%T\n", <-all)
	fmt.Printf("%T\n", <-started)
	fmt.Println((<-failures).Error())

	// Output:
	// pubsub_test.exampleStarted
	// pubsub_test.exampleFailure
	// pubsub_test.exampleStarted
	// worker unavailable
}

func ExampleWithBuffer() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	topic := pubsub.New[int]()
	values := topic.Subscribe(ctx, pubsub.WithBuffer(2))

	_ = topic.Publish(ctx, 1)
	_ = topic.Publish(ctx, 2)

	fmt.Println(<-values)
	fmt.Println(<-values)

	// Output:
	// 1
	// 2
}

func ExampleTopic_SubscriberCount() {
	ctx, cancel := context.WithCancel(context.Background())
	topic := pubsub.New[int]()
	values := topic.Subscribe(ctx)

	fmt.Println(topic.SubscriberCount())
	cancel()
	<-values // Wait for the subscription to close.
	fmt.Println(topic.SubscriberCount())

	// Output:
	// 1
	// 0
}
