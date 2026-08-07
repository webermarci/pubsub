package pubsub_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

func TestTopicPublishesToEverySubscription(t *testing.T) {
	topic := pubsub.New[int]()
	first := topic.Subscribe(t.Context())
	second := topic.Subscribe(t.Context())

	published := make(chan error, 1)
	go func() {
		published <- topic.Publish(t.Context(), 42)
	}()

	firstReceived := make(chan int, 1)
	secondReceived := make(chan int, 1)
	go func() { firstReceived <- <-first }()
	go func() { secondReceived <- <-second }()

	if got := receive(t, firstReceived); got != 42 {
		t.Fatalf("first subscriber received %d, want 42", got)
	}
	if got := receive(t, secondReceived); got != 42 {
		t.Fatalf("second subscriber received %d, want 42", got)
	}
	if err := receiveError(t, published); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
}

func TestPublishWaitsForSubscription(t *testing.T) {
	topic := pubsub.New[int]()
	subscription := topic.Subscribe(t.Context())

	published := make(chan error, 1)
	go func() {
		published <- topic.Publish(t.Context(), 42)
	}()

	select {
	case err := <-published:
		t.Fatalf("publish returned before receive, error: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	if got := receive(t, subscription); got != 42 {
		t.Fatalf("received %d, want 42", got)
	}
	if err := receiveError(t, published); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
}

func TestPublishWithNoSubscribersReturnsImmediately(t *testing.T) {
	topic := pubsub.New[int]()

	if err := topic.Publish(t.Context(), 42); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
}

func TestPublishHonorsContextWhenSubscriptionIsNotReceiving(t *testing.T) {
	topic := pubsub.New[int]()
	topic.Subscribe(t.Context())

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	if err := topic.Publish(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("publish error = %v, want deadline exceeded", err)
	}
}

func TestPublishReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	topic := pubsub.New[int]()
	if err := topic.Publish(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("publish error = %v, want canceled", err)
	}
}
