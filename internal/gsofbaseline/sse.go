package gsofbaseline

import (
	"fmt"
	"net/http"
	"time"
)

// JSONBroker fans out raw JSON messages to SSE clients (same framing as internal/gsofstats).
type JSONBroker struct {
	notify         chan []byte
	newClients     chan chan []byte
	closingClients chan chan []byte
	clients        map[chan []byte]bool
	last           []byte
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
			if len(b.last) > 0 {
				snap := append([]byte(nil), b.last...)
				select {
				case s <- snap:
				default:
				}
			}
		case s := <-b.closingClients:
			delete(b.clients, s)
		case data := <-b.notify:
			b.last = append([]byte(nil), data...)
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

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Browsers only dispatch EventSource onmessage for "data:" lines (not comments).
	fmt.Fprintf(w, "data: {\"sse\":\"open\"}\n\n")
	flusher.Flush()

	messageChan := make(chan []byte, 32)
	b.newClients <- messageChan
	defer func() { b.closingClients <- messageChan }()

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case msg := <-messageChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
