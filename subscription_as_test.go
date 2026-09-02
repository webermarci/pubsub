package pubsub_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

type testEvent interface {
	testEvent()
}

type testStarted struct {
	ID int
}

func (testStarted) testEvent() {}

type testStopped struct{}

func (testStopped) testEvent() {}

type testFailure struct {
	err error
}

func (testFailure) testEvent() {}

func (f testFailure) Error() string { return f.err.Error() }

func TestSubscribeAsFiltersConcreteTypes(t *testing.T) {
	topic := pubsub.New[testEvent]()
	started := topic.SubscribeAs[testStarted](t.Context(), pubsub.WithBuffer(1))
	stopped := topic.SubscribeAs[testStopped](t.Context())

	want := testStarted{ID: 42}
	if err := topic.Publish(t.Context(), want); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if got := receive(t, started); got != want {
		t.Fatalf("started subscription received %#v, want %#v", got, want)
	}

	select {
	case got := <-stopped:
		t.Fatalf("stopped subscription unexpectedly received %#v", got)
	default:
	}
}

func TestSubscribeStillReceivesEveryValue(t *testing.T) {
	topic := pubsub.New[testEvent]()
	all := topic.Subscribe(t.Context(), pubsub.WithBuffer(2))

	started := testStarted{ID: 1}
	stopped := testStopped{}
	if err := topic.Publish(t.Context(), started); err != nil {
		t.Fatalf("publish started: %v", err)
	}
	if err := topic.Publish(t.Context(), stopped); err != nil {
		t.Fatalf("publish stopped: %v", err)
	}

	if got := receive(t, all); got != started {
		t.Fatalf("first value = %#v, want %#v", got, started)
	}
	if got := receive(t, all); got != stopped {
		t.Fatalf("second value = %#v, want %#v", got, stopped)
	}
}

func TestSubscribeAsMatchesInterfaces(t *testing.T) {
	topic := pubsub.New[testEvent]()
	failures := topic.SubscribeAs[error](t.Context(), pubsub.WithBuffer(1))

	want := testFailure{err: errors.New("boom")}
	if err := topic.Publish(t.Context(), want); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if got := receive(t, failures); got.Error() != want.Error() {
		t.Fatalf("failure = %q, want %q", got.Error(), want.Error())
	}
}

func TestPublishReachesMultipleMatchingSubscriptionTypes(t *testing.T) {
	topic := pubsub.New[testEvent]()
	all := topic.Subscribe(t.Context(), pubsub.WithBuffer(1))
	failures := topic.SubscribeAs[testFailure](t.Context(), pubsub.WithBuffer(1))
	errorEvents := topic.SubscribeAs[error](t.Context(), pubsub.WithBuffer(1))

	want := testFailure{err: errors.New("boom")}
	if err := topic.Publish(t.Context(), want); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if got := receive(t, all); got != want {
		t.Fatalf("all subscription received %#v, want %#v", got, want)
	}
	if got := receive(t, failures); got != want {
		t.Fatalf("concrete subscription received %#v, want %#v", got, want)
	}
	if got := receive(t, errorEvents); got.Error() != want.Error() {
		t.Fatalf("interface subscription received %q, want %q", got.Error(), want.Error())
	}
}

func TestNonmatchingTypedSubscriptionDoesNotApplyBackpressure(t *testing.T) {
	topic := pubsub.New[testEvent]()
	topic.SubscribeAs[testStopped](t.Context())

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := topic.Publish(ctx, testStarted{}); err != nil {
		t.Fatalf("publish was blocked by nonmatching subscription: %v", err)
	}
}

func TestMatchingTypedSubscriptionAppliesBackpressure(t *testing.T) {
	topic := pubsub.New[testEvent]()
	topic.SubscribeAs[testStarted](t.Context())

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := topic.Publish(ctx, testStarted{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("publish error = %v, want deadline exceeded", err)
	}
}

func TestSubscribeAsWithBuffer(t *testing.T) {
	topic := pubsub.New[testEvent]()
	started := topic.SubscribeAs[testStarted](t.Context(), pubsub.WithBuffer(2))

	if err := topic.Publish(t.Context(), testStarted{ID: 1}); err != nil {
		t.Fatalf("first publish returned error: %v", err)
	}
	if err := topic.Publish(t.Context(), testStopped{}); err != nil {
		t.Fatalf("nonmatching publish returned error: %v", err)
	}
	if err := topic.Publish(t.Context(), testStarted{ID: 2}); err != nil {
		t.Fatalf("second matching publish returned error: %v", err)
	}

	if got := receive(t, started).ID; got != 1 {
		t.Fatalf("first buffered ID = %d, want 1", got)
	}
	if got := receive(t, started).ID; got != 2 {
		t.Fatalf("second buffered ID = %d, want 2", got)
	}
}

func TestCancelingSubscribeAsClosesChannelAndRemovesSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	topic := pubsub.New[testEvent]()
	started := topic.SubscribeAs[testStarted](ctx)

	if got := topic.SubscriberCount(); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}
	cancel()

	select {
	case _, ok := <-started:
		if ok {
			t.Fatal("typed subscription remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("typed subscription did not close")
	}
	if got := topic.SubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after cancellation = %d, want 0", got)
	}
}

func TestSubscriberCountIncludesEveryRequestedType(t *testing.T) {
	topic := pubsub.New[testEvent]()
	topic.Subscribe(t.Context())
	topic.SubscribeAs[testStarted](t.Context())
	topic.SubscribeAs[testStopped](t.Context())

	if got := topic.SubscriberCount(); got != 3 {
		t.Fatalf("subscriber count = %d, want 3", got)
	}
}

func TestSubscribeReceivesNilInterfaceValue(t *testing.T) {
	topic := pubsub.New[testEvent]()
	all := topic.Subscribe(t.Context(), pubsub.WithBuffer(1))

	if err := topic.Publish(t.Context(), nil); err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if got := receive(t, all); got != nil {
		t.Fatalf("received %#v, want nil", got)
	}
}

func TestConcurrentTypedSubscriptionCancellationAndPublishing(t *testing.T) {
	topic := pubsub.New[testEvent]()
	const iterations = 200

	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for range iterations {
			ctx, cancel := context.WithCancel(t.Context())
			topic.SubscribeAs[testStopped](ctx)
			cancel()
		}
	}()
	go func() {
		defer group.Done()
		for range iterations {
			if err := topic.Publish(t.Context(), testStarted{}); err != nil {
				t.Errorf("publish returned error: %v", err)
				return
			}
		}
	}()
	group.Wait()
}
