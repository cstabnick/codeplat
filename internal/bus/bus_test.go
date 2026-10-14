package bus_test

import (
	"sync"
	"testing"
	"time"

	"codeplat/internal/bus"
)

// receive returns the next message on ch, or fails the test if none arrives.
func receive(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case data := <-ch:
		return data
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a message")
		return nil
	}
}

// expectNothing fails the test if ch holds a message right now.
func expectNothing(t *testing.T, ch <-chan []byte) {
	t.Helper()
	select {
	case data := <-ch:
		t.Fatalf("unexpected message %q", data)
	default:
	}
}

func TestSubscriberReceivesOnlyItsTopic(t *testing.T) {
	b := bus.New()
	ch, cancel := b.Subscribe("alpha")
	defer cancel()

	b.Publish("beta", []byte("other"))
	b.Publish("alpha", []byte("hello"))

	if got := string(receive(t, ch)); got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
	expectNothing(t, ch)
}

func TestTwoSubscribersOfOneTopicBothReceive(t *testing.T) {
	b := bus.New()
	ch1, cancel1 := b.Subscribe("topic")
	defer cancel1()
	ch2, cancel2 := b.Subscribe("topic")
	defer cancel2()

	b.Publish("topic", []byte("msg"))

	if got := string(receive(t, ch1)); got != "msg" {
		t.Fatalf("first subscriber got %q, want %q", got, "msg")
	}
	if got := string(receive(t, ch2)); got != "msg" {
		t.Fatalf("second subscriber got %q, want %q", got, "msg")
	}
}

func TestPublishWithNoSubscribersDoesNothing(t *testing.T) {
	b := bus.New()
	done := make(chan struct{})
	go func() {
		b.Publish("nobody", []byte("lost"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish with no subscribers did not return")
	}

	// A message published before anyone subscribed is not stored for later.
	ch, cancel := b.Subscribe("nobody")
	defer cancel()
	expectNothing(t, ch)
}

func TestPublishDoesNotBlockOnSlowSubscriber(t *testing.T) {
	b := bus.New()
	ch, cancel := b.Subscribe("slow")
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish("slow", []byte{byte(i)})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a subscriber that never reads")
	}

	if n := len(ch); n != 16 {
		t.Fatalf("buffered messages = %d, want 16", n)
	}
}

func TestSlowSubscriberDoesNotStarveOthers(t *testing.T) {
	b := bus.New()
	_, cancelSlow := b.Subscribe("topic")
	defer cancelSlow()
	fast, cancelFast := b.Subscribe("topic")
	defer cancelFast()

	for i := 0; i < 100; i++ {
		b.Publish("topic", []byte{byte(i)})
		// Drain the fast subscriber so its buffer never fills.
		if got := receive(t, fast); len(got) != 1 || got[0] != byte(i) {
			t.Fatalf("fast subscriber got %v at step %d", got, i)
		}
	}
}

func TestCancelStopsDelivery(t *testing.T) {
	b := bus.New()
	ch, cancel := b.Subscribe("topic")
	cancel()

	b.Publish("topic", []byte("after cancel"))
	expectNothing(t, ch)
}

func TestCancelLeavesOtherSubscribersActive(t *testing.T) {
	b := bus.New()
	ch1, cancel1 := b.Subscribe("topic")
	ch2, cancel2 := b.Subscribe("topic")
	defer cancel2()

	cancel1()
	b.Publish("topic", []byte("still here"))

	expectNothing(t, ch1)
	if got := string(receive(t, ch2)); got != "still here" {
		t.Fatalf("remaining subscriber got %q, want %q", got, "still here")
	}
}

func TestCancelTwiceDoesNotPanic(t *testing.T) {
	b := bus.New()
	ch, cancel := b.Subscribe("topic")

	cancel()
	cancel()

	b.Publish("topic", []byte("x"))
	expectNothing(t, ch)
}

func TestCancelDoesNotCloseChannel(t *testing.T) {
	b := bus.New()
	ch, cancel := b.Subscribe("topic")
	cancel()

	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("cancel closed the channel")
		}
		t.Fatal("unexpected message after cancel")
	default:
	}
}

func TestConcurrentPublishSubscribeCancel(t *testing.T) {
	b := bus.New()
	const workers = 8
	const rounds = 200

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				b.Publish("shared", []byte("payload"))
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				ch, cancel := b.Subscribe("shared")
				select {
				case <-ch:
				default:
				}
				cancel()
				cancel()
			}
		}()
	}
	wg.Wait()

	// After all subscriptions are cancelled the topic must be gone and
	// publishing must still be harmless.
	ch, cancel := b.Subscribe("shared")
	defer cancel()
	b.Publish("shared", []byte("final"))
	if got := string(receive(t, ch)); got != "final" {
		t.Fatalf("got %q, want %q", got, "final")
	}
}
