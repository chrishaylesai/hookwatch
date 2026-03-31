package hub

import (
	"testing"
	"time"
)

func TestPublishDeliversToSubscribersForMatchingToken(t *testing.T) {
	t.Parallel()

	eventHub := New()
	tokenID := "token-1"
	ch := eventHub.Subscribe(tokenID)
	t.Cleanup(func() {
		eventHub.Unsubscribe(tokenID, ch)
	})

	event := Event{ID: "1", Type: "request.created", Data: []byte(`{"ok":true}`)}
	eventHub.Publish(tokenID, event)

	select {
	case got := <-ch:
		if got.ID != event.ID {
			t.Fatalf("event ID = %q, want %q", got.ID, event.ID)
		}
		if got.Type != event.Type {
			t.Fatalf("event Type = %q, want %q", got.Type, event.Type)
		}
		if string(got.Data) != string(event.Data) {
			t.Fatalf("event Data = %q, want %q", string(got.Data), string(event.Data))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published event")
	}
}

func TestPublishSkipsSubscribersForOtherTokens(t *testing.T) {
	t.Parallel()

	eventHub := New()
	matching := eventHub.Subscribe("token-1")
	other := eventHub.Subscribe("token-2")
	t.Cleanup(func() {
		eventHub.Unsubscribe("token-1", matching)
		eventHub.Unsubscribe("token-2", other)
	})

	eventHub.Publish("token-1", Event{ID: "1"})

	select {
	case <-matching:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for matching subscriber")
	}

	select {
	case got := <-other:
		t.Fatalf("unexpected event for other token: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUnsubscribeStopsFutureDelivery(t *testing.T) {
	t.Parallel()

	eventHub := New()
	tokenID := "token-1"
	ch := eventHub.Subscribe(tokenID)

	eventHub.Unsubscribe(tokenID, ch)
	eventHub.Publish(tokenID, Event{ID: "after-unsubscribe"})

	select {
	case got := <-ch:
		t.Fatalf("unexpected event after unsubscribe: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPublishDropsEventsForSlowSubscribers(t *testing.T) {
	t.Parallel()

	eventHub := New()
	tokenID := "token-1"
	ch := eventHub.Subscribe(tokenID)
	t.Cleanup(func() {
		eventHub.Unsubscribe(tokenID, ch)
	})

	for i := 0; i < cap(ch)+1; i++ {
		eventHub.Publish(tokenID, Event{ID: "1"})
	}

	if len(ch) != cap(ch) {
		t.Fatalf("buffer length = %d, want %d", len(ch), cap(ch))
	}
}
