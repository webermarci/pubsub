package pubsub_test

import (
	"context"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

func BenchmarkPublish(b *testing.B) {
	topic := pubsub.New[int]()
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	subscription := topic.Subscribe(ctx)
	go func() {
		for range subscription {
		}
	}()

	b.ResetTimer()
	for b.Loop() {
		if err := topic.Publish(ctx, 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublish_Contention(b *testing.B) {
	topic := pubsub.New[int]()
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	for range 10 {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				subscriptionCtx, subscriptionCancel := context.WithCancel(ctx)
				subscription := topic.Subscribe(subscriptionCtx)
				go func() {
					for range subscription {
					}
				}()
				time.Sleep(10 * time.Microsecond)
				subscriptionCancel()
			}
		}()
	}

	b.ResetTimer()
	for b.Loop() {
		if err := topic.Publish(ctx, 1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublish_FanOut100(b *testing.B) {
	topic := pubsub.New[int]()
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	for range 100 {
		subscription := topic.Subscribe(ctx)
		go func() {
			for range subscription {
			}
		}()
	}

	b.ResetTimer()
	for b.Loop() {
		if err := topic.Publish(ctx, 1); err != nil {
			b.Fatal(err)
		}
	}
}
