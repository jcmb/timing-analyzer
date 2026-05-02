package gsofstats

import (
	"fmt"
	"net/http"
)

// JSONBroker fans out raw JSON messages to SSE clients (same framing as internal/telemetry).
type JSONBroker struct {
	notify         chan []byte
	newClients     chan chan []byte
	closingClients chan chan []byte
	clients        map[chan []byte]bool
}

func NewJSONBroker() *JSONBroker {
	b := &JSONBroker{
		notify:         make(chan []byte, 4),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}
	go b.listen()
	return b
}

func (b *JSONBroker) listen() {
	for {
		select {
		case s := <-b.newClients:
			b.clients[s] = true
		case s := <-b.closingClients:
			delete(b.clients, s)
		case data := <-b.notify:
			for ch := range b.clients {
				select {
				case ch <- data:
				default:
				}
			}
		}
	}
}

// Publish sends one JSON payload to all connected SSE clients (drops if congested).
func (b *JSONBroker) Publish(data []byte) {
	select {
	case b.notify <- data:
	default:
	}
}

// ServeHTTP implements text/event-stream for browser EventSource.
func (b *JSONBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	padding := make([]byte, 2048)
	for i := range padding {
		padding[i] = ' '
	}
	fmt.Fprintf(w, ":%s\n\n", string(padding))
	flusher.Flush()

	messageChan := make(chan []byte, 8)
	b.newClients <- messageChan
	defer func() { b.closingClients <- messageChan }()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-messageChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
