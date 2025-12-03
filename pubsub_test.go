package pubsub

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBasicPubSub(t *testing.T) {
	pubsub := New[string]()

	topic := "greet"
	sub := pubsub.Subscribe(t.Context(), topic)

	expected := "hello"
	pubsub.Publish(topic, expected)

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
	pubsub := New[int]().WithCapacity(5)

	topic := "updates"
	sub1 := pubsub.Subscribe(t.Context(), topic)
	sub2 := pubsub.Subscribe(t.Context(), topic)

	value := 42
	pubsub.Publish(topic, value)

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
	ps := New[string]()

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

func TestCleanupAndMemoryLeak(t *testing.T) {
	ps := New[int]()

	ctx, cancel := context.WithCancel(t.Context())

	topic := "leaks"
	sub := ps.Subscribe(ctx, topic)

	ps.mutex.RLock()
	if len(ps.subscribers[topic]) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(ps.subscribers[topic]))
	}
	ps.mutex.RUnlock()

	cancel()

	select {
	case _, ok := <-sub:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for channel close")
	}

	success := false
	for range 10 {
		ps.mutex.RLock()
		_, exists := ps.subscribers[topic]
		ps.mutex.RUnlock()

		if !exists {
			success = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !success {
		t.Error("map key was not deleted after last subscriber left")
	}
}

func TestNonBlocking(t *testing.T) {
	ps := New[int]()

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

func TestConcurrencyRace(t *testing.T) {
	ps := New[int]().WithCapacity(10)
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

func TestPanicOnNilContext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()

	ps := New[string]()
	ps.Subscribe(nil, "topic")
}

func BenchmarkPublish_SingleSubscriber(b *testing.B) {
	ps := New[int]().WithCapacity(b.N)
	sub := ps.Subscribe(b.Context(), "bench")

	go func() {
		for range sub {
		}
	}()

	for i := 0; b.Loop(); i++ {
		ps.Publish("bench", i)
	}
}

func BenchmarkPublish_FanOut100(b *testing.B) {
	ps := New[int]().WithCapacity(b.N)

	for range 100 {
		sub := ps.Subscribe(b.Context(), "bench")
		go func(c <-chan int) {
			for range c {
			}
		}(sub)
	}

	for i := 0; b.Loop(); i++ {
		ps.Publish("bench", i)
	}
}

func BenchmarkPublish_Contention(b *testing.B) {
	ps := New[int]().WithCapacity(100)
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

	sub := ps.Subscribe(ctx, "bench")
	go func() {
		for range sub {
		}
	}()

	for i := 0; b.Loop(); i++ {
		ps.Publish("bench", i)
	}
}
