package telemetry

import (
	"encoding/json"
	"fmt"
	"net/http"

	"timing-analyzer/internal/core"
)

type SSEBroker struct {
	// Notifier receives events from the Timing Engine
	Notifier       chan core.TelemetryEvent
	newClients     chan chan []byte
	closingClients chan chan []byte
	clients        map[chan []byte]bool
}

func NewSSEBroker() *SSEBroker {
	broker := &SSEBroker{
		Notifier:       make(chan core.TelemetryEvent, 100),
		newClients:     make(chan chan []byte),
		closingClients: make(chan chan []byte),
		clients:        make(map[chan []byte]bool),
	}
	go broker.listen()
	return broker
}

func (broker *SSEBroker) listen() {
	for {
		select {
		case s := <-broker.newClients:
			broker.clients[s] = true
		case s := <-broker.closingClients:
			delete(broker.clients, s)
		case event := <-broker.Notifier:
			data, _ := json.Marshal(event)
			for clientMessageChan := range broker.clients {
				select {
				case clientMessageChan <- data:
				default: // Drop message if the client's network is too slow, keeps server healthy
				}
			}
		}
	}
}

func (broker *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Ensure the web server supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// 1. Write the 200 OK header and flush IMMEDIATELY to trigger JS onopen
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 2. The Safari/WebKit Fix: Send a 2KB comment padding.
	// SSE ignores any line starting with a colon (it's treated as a comment).
	// This instantly fills the browser's MIME-sniffing buffer so it processes live data immediately.
	padding := make([]byte, 2048)
	for i := range padding {
		padding[i] = ' '
	}
	fmt.Fprintf(w, ":%s\n\n", string(padding))
	flusher.Flush()

	messageChan := make(chan []byte)
	broker.newClients <- messageChan

	defer func() {
		broker.closingClients <- messageChan
	}()

	for {
		select {
		case <-r.Context().Done():
			return // Client closed the browser tab
		case msg := <-messageChan:
			// 3. Format as SSE data and ALWAYS include the double newline \n\n
			fmt.Fprintf(w, "data: %s\n\n", msg)
			// 4. Force Go to push the bytes down the TCP socket immediately
			flusher.Flush()
		}
	}
}
