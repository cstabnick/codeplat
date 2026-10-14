package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"codeplat/internal/bus"
	"codeplat/internal/core"
)

// topicsFor returns the bus topics an event is published on: always
// "project.<ProjectKey>", and also "issue.<IssueID>" when the event has an
// issue ID.
func topicsFor(e core.Event) []string {
	topics := []string{"project." + e.ProjectKey}
	if e.IssueID != "" {
		topics = append(topics, "issue."+e.IssueID)
	}
	return topics
}

// feed publishes every event with an ID greater than after onto b, checking
// for new events every interval, until ctx is done.
func feed(ctx context.Context, svc *core.Service, b *bus.Bus, after int64, interval time.Duration, log *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		events, err := svc.EventsSince(ctx, after)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Error("reading events", "after", after, "err", err)
			continue
		}
		for _, e := range events {
			data, err := json.Marshal(e)
			if err != nil {
				log.Error("encoding event", "event_id", e.ID, "err", err)
			} else {
				for _, topic := range topicsFor(e) {
					b.Publish(topic, data)
				}
			}
			after = e.ID
		}
	}
}
