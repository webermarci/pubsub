package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

func TestBufferedSubscriptionAbsorbsBurstUntilFull(t *testing.T) {
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(t.Context(), pubsub.WithBuffer(2))

	if err := topic.Publish(t.Context(), 1); err != nil {
		t.Fatalf("first publish returned error: %v", err)
	}
	if err := topic.Publish(t.Context(), 2); err != nil {
		t.Fatalf("second publish returned error: %v", err)
	}

	publishCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	published := make(chan error, 1)
	go func() {
		published <- topic.Publish(publishCtx, 3)
	}()

	select {
	case err := <-published:
		t.Fatalf("third publish returned before the buffer had room, error: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	if got := receive(t, subscription); got != 1 {
		t.Fatalf("first buffered value = %d, want 1", got)
	}
	if err := receiveError(t, published); err != nil {
		t.Fatalf("third publish returned error: %v", err)
	}
	if got := receive(t, subscription); got != 2 {
		t.Fatalf("second buffered value = %d, want 2", got)
	}
	if got := receive(t, subscription); got != 3 {
		t.Fatalf("third buffered value = %d, want 3", got)
	}
}

func TestBufferedSubscriptionDrainsQueuedValuesAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(ctx, pubsub.WithBuffer(2))

	if err := topic.Publish(t.Context(), 1); err != nil {
		t.Fatalf("first publish returned error: %v", err)
	}
	if err := topic.Publish(t.Context(), 2); err != nil {
		t.Fatalf("second publish returned error: %v", err)
	}

	cancel()
	if got := receive(t, subscription); got != 1 {
		t.Fatalf("first queued value = %d, want 1", got)
	}
	if got := receive(t, subscription); got != 2 {
		t.Fatalf("second queued value = %d, want 2", got)
	}

	select {
	case _, ok := <-subscription:
		if ok {
			t.Fatal("subscription remained open after queued values were drained")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not close after queued values were drained")
	}
}

func TestWithBufferRejectsNegativeSize(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("WithBuffer did not reject a negative size")
		}
	}()

	pubsub.WithBuffer(-1)
}

func TestSubscribeRejectsNilOption(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Subscribe did not reject a nil option")
		}
	}()

	topic := pubsub.New[int]()
	topic.Subscribe(t.Context(), nil)
}

func TestSubscriptionClosesWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(ctx)

	if topic.SubscriberCount() != 1 {
		t.Fatalf("subscriber count = %d, want 1", topic.SubscriberCount())
	}

	cancel()

	select {
	case _, ok := <-subscription:
		if ok {
			t.Fatal("subscription value channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not close after context cancellation")
	}

	if topic.SubscriberCount() != 0 {
		t.Fatalf("subscriber count = %d after cancellation, want 0", topic.SubscriberCount())
	}
}

func TestSubscribeWithCanceledContextReturnsClosedChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	topic := pubsub.New[int]()
	subscription := topic.Subscribe(ctx)

	if _, ok := <-subscription; ok {
		t.Fatal("expected subscription channel to be closed")
	}
	if topic.SubscriberCount() != 0 {
		t.Fatalf("subscriber count = %d, want 0", topic.SubscriberCount())
	}
}

func TestCancelingSubscriptionUnblocksPublish(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	topic := pubsub.New[int]()
	topic.Subscribe(ctx)

	started := make(chan struct{})
	published := make(chan error, 1)
	go func() {
		close(started)
		published <- topic.Publish(context.Background(), 42)
	}()
	<-started

	select {
	case err := <-published:
		t.Fatalf("publish returned before subscription cancellation, error: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	cancel()
	if err := receiveError(t, published); err != nil {
		t.Fatalf("publish returned error after subscription cancellation: %v", err)
	}
}
