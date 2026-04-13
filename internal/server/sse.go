package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Event represents a server-sent event.
type Event struct {
	Type      string `json:"type"`
	Data      any    `json:"data,omitempty"`
	Timestamp string `json:"timestamp"`
}

// NewEvent creates an event with the current timestamp.
func NewEvent(eventType string, data any) Event {
	return Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// SSEHub manages Server-Sent Event connections.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
}

type sseClient struct {
	events chan Event
	done   chan struct{}
	admin  bool // admin clients receive all events
}

func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[*sseClient]struct{}),
	}
}

// Broadcast sends an event to all connected clients.
func (h *SSEHub) Broadcast(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.events <- e:
		default:
			// Client buffer full, skip this event
		}
	}
}

// BroadcastAdmin sends an event only to admin clients.
func (h *SSEHub) BroadcastAdmin(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		if c.admin {
			select {
			case c.events <- e:
			default:
			}
		}
	}
}

// ServeHTTP handles SSE connections. Set isAdmin=true for admin event streams.
func (h *SSEHub) Handler(isAdmin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		client := &sseClient{
			events: make(chan Event, 32),
			done:   make(chan struct{}),
			admin:  isAdmin,
		}

		h.mu.Lock()
		h.clients[client] = struct{}{}
		h.mu.Unlock()

		defer func() {
			h.mu.Lock()
			delete(h.clients, client)
			h.mu.Unlock()
			close(client.done)
		}()

		// Send initial connection event
		writeSSE(w, flusher, NewEvent("connected", nil))

		// Keep-alive ticker
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-client.events:
				writeSSE(w, flusher, e)
			case <-ticker.C:
				// Send comment as keep-alive
				fmt.Fprintf(w, ": keepalive\n\n")
				flusher.Flush()
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		log.Printf("SSE marshal error: %v", err)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
	flusher.Flush()
}
