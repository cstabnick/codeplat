// Package sse streams the messages of one topic to a browser as
// Server-Sent Events.
package sse

import (
	"bytes"
	"net/http"
	"time"
)

// Subscriber is the source of messages. A message bus satisfies it.
type Subscriber interface {
	// Subscribe returns a channel of the messages for topic and a function
	// that ends the subscription.
	Subscribe(topic string) (<-chan []byte, func())
}

// keepAlive is how often a comment line is sent on an idle stream so that
// proxies keep the connection open. It is a variable so tests can shorten it.
var keepAlive = 25 * time.Second

// Handler returns an http.Handler that keeps the response open and sends
// each message published to the topic named by the "topic" query parameter.
func Handler(sub Subscriber) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			http.Error(w, "topic is required", http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		messages, cancel := sub.Subscribe(topic)
		defer cancel()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(": connected\n\n"))
		flusher.Flush()

		ticker := time.NewTicker(keepAlive)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg, open := <-messages:
				if !open {
					return
				}
				writeEvent(w, msg)
				flusher.Flush()
			case <-ticker.C:
				_, _ = w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			}
		}
	})
}

// writeEvent writes one "changed" event whose data is msg. Each line of msg
// becomes its own data line, as the SSE format requires.
func writeEvent(w http.ResponseWriter, msg []byte) {
	var b bytes.Buffer
	b.WriteString("event: changed\n")
	for _, line := range bytes.Split(msg, []byte("\n")) {
		b.WriteString("data: ")
		b.Write(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	_, _ = w.Write(b.Bytes())
}
