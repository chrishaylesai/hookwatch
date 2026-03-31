package hub

import "sync"

// Hub is an in-process pub/sub hub for SSE events.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan Event]struct{}
}

// Event represents an SSE event to be broadcast to subscribers.
type Event struct {
	ID   string
	Type string
	Data []byte
}

// New creates a new Hub.
func New() *Hub {
	return &Hub{
		subscribers: make(map[string]map[chan Event]struct{}),
	}
}

// Subscribe returns a channel that receives events for the given token ID.
func (h *Hub) Subscribe(tokenID string) chan Event {
	ch := make(chan Event, 64)
	h.mu.Lock()
	if h.subscribers[tokenID] == nil {
		h.subscribers[tokenID] = make(map[chan Event]struct{})
	}
	h.subscribers[tokenID][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (h *Hub) Unsubscribe(tokenID string, ch chan Event) {
	h.mu.Lock()
	if subs, ok := h.subscribers[tokenID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.subscribers, tokenID)
		}
	}
	h.mu.Unlock()
}

// Publish sends an event to all subscribers of a token.
func (h *Hub) Publish(tokenID string, event Event) {
	h.mu.RLock()
	subs := h.subscribers[tokenID]
	if len(subs) == 0 {
		h.mu.RUnlock()
		return
	}
	channels := make([]chan Event, 0, len(subs))
	for ch := range subs {
		channels = append(channels, ch)
	}
	h.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// Drop event if subscriber is too slow.
		}
	}
}
