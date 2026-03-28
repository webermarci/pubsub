package pubsub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

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

func BenchmarkPublish_SingleSubscriber(b *testing.B) {
	ps := pubsub.New[string, int](100)
	defer ps.Close()
	sub := ps.Subscribe(b.Context(), "bench")

	go func() {
		for range sub {
		}
	}()

	for i := 0; b.Loop(); i++ {
		ps.Publish("bench", i)
	}
}

func BenchmarkPublish_RunParallel(b *testing.B) {
	ps := pubsub.New[string, int](100)
	defer ps.Close()

	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()
	sub := ps.Subscribe(ctx, "bench")
	go func() {
		for range sub {
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ps.Publish("bench", 1)
		}
	})
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

	for i := 0; b.Loop(); i++ {
		ps.Publish("bench", i)
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

	sub := ps.Subscribe(ctx, "bench")
	go func() {
		for range sub {
		}
	}()

	for i := 0; b.Loop(); i++ {
		ps.Publish("bench", i)
	}
}
