package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"timing-analyzer/internal/core"
)

type SSEBroker struct {
	Notifier       chan core.TelemetryEvent
	newClients     chan chan core.TelemetryEvent
	closingClients chan chan core.TelemetryEvent
	clients        map[chan core.TelemetryEvent]bool
}

func NewSSEBroker() *SSEBroker {
	broker := &SSEBroker{
		Notifier:       make(chan core.TelemetryEvent, 10),
		newClients:     make(chan chan core.TelemetryEvent),
		closingClients: make(chan chan core.TelemetryEvent),
		clients:        make(map[chan core.TelemetryEvent]bool),
	}
	go broker.listen()
	return broker
}

func (b *SSEBroker) listen() {
	for {
		select {
		case s := <-b.newClients:
			b.clients[s] = true
			slog.Info("Web UI Client Connected", "active_clients", len(b.clients))
		case s := <-b.closingClients:
			delete(b.clients, s)
			slog.Info("Web UI Client Disconnected", "active_clients", len(b.clients))
		case event := <-b.Notifier:
			for clientMessageChan := range b.clients {
				select {
				case clientMessageChan <- event:
				default:
				}
			}
		}
	}
}

func (b *SSEBroker) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := rw.(http.Flusher)
	if !ok {
		http.Error(rw, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan core.TelemetryEvent)
	b.newClients <- messageChan

	defer func() {
		b.closingClients <- messageChan
	}()

	notify := req.Context().Done()
	for {
		select {
		case <-notify:
			return
		case event := <-messageChan:
			jsonData, _ := json.Marshal(event)
			fmt.Fprintf(rw, "data: %s\n\n", jsonData)
			flusher.Flush()
		}
	}
}
