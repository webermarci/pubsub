package pubsub_test

import (
	"sync"
	"testing"
	"time"

	"github.com/webermarci/pubsub"
)

func TestConcurrentPublishing(t *testing.T) {
	const (
		publishers    = 8
		valuesPerCopy = 100
	)

	topic := pubsub.New[int]()
	subscription := topic.Subscribe(t.Context())

	const total = publishers * valuesPerCopy
	received := make(chan struct{})
	go func() {
		for range total {
			<-subscription
		}
		close(received)
	}()

	var group sync.WaitGroup
	group.Add(publishers)
	for range publishers {
		go func() {
			defer group.Done()
			for value := range valuesPerCopy {
				if err := topic.Publish(t.Context(), value); err != nil {
					t.Errorf("publish returned error: %v", err)
					return
				}
			}
		}()
	}

	group.Wait()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive all published values")
	}
}

func receive[T any](t testing.TB, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		var zero T
		t.Fatal("timed out waiting for subscription value")
		return zero
	}
}

func receiveError(t testing.TB, results <-chan error) error {
	t.Helper()
	select {
	case err := <-results:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for publish result")
		return nil
	}
}
