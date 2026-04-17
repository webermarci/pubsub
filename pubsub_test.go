package pubsub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

// mockObserver tracks calls to verify the Observer interface is wired correctly.
type mockObserver struct {
	mu           sync.Mutex
	published    int
	dropped      int
	subscribed   int
	unsubscribed int
	closed       bool
}

func (m *mockObserver) OnPublish(topic string, payload int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published++
}
func (m *mockObserver) OnDropped(topic string, payload int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropped++
}
func (m *mockObserver) OnSubscribed(topic string) { m.mu.Lock(); defer m.mu.Unlock(); m.subscribed++ }
func (m *mockObserver) OnUnsubscribed(topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsubscribed++
}
func (m *mockObserver) OnClosed() { m.mu.Lock(); defer m.mu.Unlock(); m.closed = true }

func TestBasicPubSub(t *testing.T) {
	ps := pubsub.New[string, string](10)
	defer ps.Close()

	topic := "greet"
	sub := ps.Subscribe(t.Context(), topic)

	expected := "hello"
	ps.Publish(topic, expected)

	select {
	case message := <-sub:
		if message != expected {
			t.Errorf("expected %s, got %s", expected, message)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestFanOut(t *testing.T) {
	ps := pubsub.New[string, int](5)
	defer ps.Close()

	topic := "updates"
	sub1 := ps.Subscribe(t.Context(), topic)
	sub2 := ps.Subscribe(t.Context(), topic)

	value := 42
	ps.Publish(topic, value)

	read := func(ch <-chan int) int {
		select {
		case v := <-ch:
			return v
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message")
			return 0
		}
	}

	if v := read(sub1); v != value {
		t.Errorf("sub1 expected %d, got %d", value, v)
	}
	if v := read(sub2); v != value {
		t.Errorf("sub2 expected %d, got %d", value, v)
	}
}

func TestTopicSeparation(t *testing.T) {
	ps := pubsub.New[string, string](1)
	defer ps.Close()

	subA := ps.Subscribe(t.Context(), "topicA")
	subB := ps.Subscribe(t.Context(), "topicB")

	ps.Publish("topicA", "msgA")

	select {
	case msg := <-subA:
		if msg != "msgA" {
			t.Errorf("expected msgA, got %s", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for topicA")
	}

	select {
	case msg := <-subB:
		t.Errorf("subB received unexpected message: %s", msg)
	default:
	}
}

func TestNonBlocking(t *testing.T) {
	ps := pubsub.New[string, int](1)
	defer ps.Close()

	topic := "fast"
	sub := ps.Subscribe(t.Context(), topic)

	ps.Publish(topic, 1)

	done := make(chan bool)
	go func() {
		ps.Publish(topic, 2)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on full channel")
	}

	if v := <-sub; v != 1 {
		t.Errorf("expected 1, got %d", v)
	}
}

func TestSliceShrinking(t *testing.T) {
	ps := pubsub.New[string, int](1)
	defer ps.Close()
	topic := "shrink"

	count := 100
	cancels := make([]context.CancelFunc, count)

	for i := range count {
		ctx, cancel := context.WithCancel(t.Context())
		ps.Subscribe(ctx, topic)
		cancels[i] = cancel
	}

	for i := range 80 {
		cancels[i]()
	}

	// Give AfterFunc goroutines time to run
	time.Sleep(50 * time.Millisecond)
	ps.Publish(topic, 999)
}

func TestNoisyNeighbor(t *testing.T) {
	ps := pubsub.New[string, int](1)
	defer ps.Close()
	topic := "noisy"

	// Sub 1: Will be full
	ps.Subscribe(t.Context(), topic)
	// Sub 2: We will read from this one
	fastSub := ps.Subscribe(t.Context(), topic)

	ps.Publish(topic, 1)   // Buffers now [1]
	ps.Publish(topic, 100) // Buffers full, 100 is dropped for both

	// Empty fastSub so it has room again
	<-fastSub

	ps.Publish(topic, 200)

	select {
	case v := <-fastSub:
		if v != 200 {
			t.Errorf("expected 200, got %d", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Fast consumer was blocked or message was dropped unnecessarily")
	}
}

func TestObserverHooks(t *testing.T) {
	obs := &mockObserver{}
	ps := pubsub.New[string, int](0, pubsub.WithObserver(obs))

	topic := "obs-test"
	ctx, cancel := context.WithCancel(t.Context())

	// 1. Test Subscribed
	ps.Subscribe(ctx, topic)

	// 2. Test Publish & Dropped
	ps.Publish(topic, 42)

	// 3. Test Unsubscribed
	cancel()
	time.Sleep(50 * time.Millisecond) // wait for cleanup

	// 4. Test Closed
	ps.Close()

	obs.mu.Lock()
	defer obs.mu.Unlock()

	if obs.subscribed != 1 {
		t.Errorf("expected 1 subscribed, got %d", obs.subscribed)
	}
	if obs.published != 1 {
		t.Errorf("expected 1 published, got %d", obs.published)
	}
	if obs.dropped != 1 {
		t.Errorf("expected 1 dropped, got %d", obs.dropped)
	}
	if obs.unsubscribed != 1 {
		t.Errorf("expected 1 unsubscribed, got %d", obs.unsubscribed)
	}
	if !obs.closed {
		t.Error("expected OnClosed to be called")
	}
}

func TestSubscribeWithCanceledContext(t *testing.T) {
	ps := pubsub.New[string, int](10)
	defer ps.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	sub := ps.Subscribe(ctx, "dead")

	_, ok := <-sub
	if ok {
		t.Error("expected channel to be closed immediately for canceled context")
	}
}

func TestClose(t *testing.T) {
	ps := pubsub.New[string, int](5)

	topic := "shutdown"
	sub1 := ps.Subscribe(t.Context(), topic)
	sub2 := ps.Subscribe(t.Context(), topic)

	ps.Close()

	select {
	case _, ok := <-sub1:
		if ok {
			t.Error("expected sub1 to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sub1 to close")
	}

	select {
	case _, ok := <-sub2:
		if ok {
			t.Error("expected sub2 to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for sub2 to close")
	}

	ps.Publish(topic, 42)

	sub3 := ps.Subscribe(t.Context(), "new-topic")
	if _, ok := <-sub3; ok {
		t.Error("expected new subscription after close to be closed immediately")
	}
}

func TestConcurrencyRace(t *testing.T) {
	ps := pubsub.New[string, int](1)
	defer ps.Close()
	var wg sync.WaitGroup

	for range 50 {
		wg.Go(func() {
			for j := range 100 {
				ps.Publish("race", j)
			}
		})
	}

	for range 50 {
		wg.Go(func() {
			ctx, cancel := context.WithCancel(t.Context())
			sub := ps.Subscribe(ctx, "race")

			go func() {
				for range sub {
				}
			}()

			time.Sleep(time.Millisecond)
			cancel()
		})
	}

	wg.Wait()
}

func BenchmarkPublish_NoObserver(b *testing.B) {
	ps := pubsub.New[string, int](100)
	defer ps.Close()
	sub := ps.Subscribe(b.Context(), "bench")

	go func() {
		for range sub {
		}
	}()

	b.ResetTimer()
	for b.Loop() {
		ps.Publish("bench", 1)
	}
}

func BenchmarkPublish_WithObserver(b *testing.B) {
	ps := pubsub.New[string, int](100, pubsub.WithObserver(&mockObserver{}))
	defer ps.Close()
	sub := ps.Subscribe(b.Context(), "bench")

	go func() {
		for range sub {
		}
	}()

	b.ResetTimer()
	for b.Loop() {
		ps.Publish("bench", 1)
	}
}

func BenchmarkPublish_Contention(b *testing.B) {
	ps := pubsub.New[string, int](100)
	defer ps.Close()
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	for range 10 {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
					subCtx, subCancel := context.WithCancel(ctx)
					sub := ps.Subscribe(subCtx, "bench")
					go func() {
						for range sub {
						}
					}()
					time.Sleep(10 * time.Microsecond)
					subCancel()
				}
			}
		}()
	}

	b.ResetTimer()
	for b.Loop() {
		ps.Publish("bench", 1)
	}
}

func BenchmarkPublish_FanOut100(b *testing.B) {
	ps := pubsub.New[string, int](100)
	defer ps.Close()

	for range 100 {
		sub := ps.Subscribe(b.Context(), "bench")
		go func(c <-chan int) {
			for range c {
			}
		}(sub)
	}

	b.ResetTimer()
	for b.Loop() {
		ps.Publish("bench", 1)
	}
}
