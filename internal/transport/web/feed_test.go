package web

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"codeplat/internal/apptest"
	"codeplat/internal/bus"
	"codeplat/internal/core"
)

// waitTimeout bounds every wait for a message or for feed to return.
const waitTimeout = 2 * time.Second

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recvEvent waits for one message on ch and decodes it as an Event.
func recvEvent(t *testing.T, ch <-chan []byte) core.Event {
	t.Helper()
	select {
	case data := <-ch:
		var e core.Event
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatalf("message does not unmarshal to core.Event: %v (%s)", err, data)
		}
		return e
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for a message")
	}
	return core.Event{}
}

func TestTopicsFor(t *testing.T) {
	tests := []struct {
		name string
		e    core.Event
		want []string
	}{
		{
			name: "project event has only the project topic",
			e:    core.Event{Type: core.EventProjectCreated, ProjectKey: "BOOK"},
			want: []string{"project.BOOK"},
		},
		{
			name: "issue event also has the issue topic",
			e:    core.Event{Type: core.EventCommentAdded, ProjectKey: "BOOK", IssueID: "BOOK-3"},
			want: []string{"project.BOOK", "issue.BOOK-3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topicsFor(tt.e)
			if len(got) != len(tt.want) {
				t.Fatalf("topicsFor() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("topicsFor() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestFeedSkipsExistingEvents(t *testing.T) {
	a := apptest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "BOOK", Name: "Books"}); err != nil {
		t.Fatal(err)
	}
	last, err := a.Service.LastEventID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	b := bus.New()
	ch, unsub := b.Subscribe("project.BOOK")
	defer unsub()

	done := make(chan struct{})
	go func() {
		feed(ctx, a.Service, b, last, 5*time.Millisecond, discardLog())
		close(done)
	}()

	// The first delivered event must be the issue, not the replayed project.
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "BOOK", Title: "First"}); err != nil {
		t.Fatal(err)
	}
	e := recvEvent(t, ch)
	if e.Type != core.EventIssueCreated {
		t.Fatalf("first delivered event type = %q, want %q (project.created was replayed)", e.Type, core.EventIssueCreated)
	}

	cancel()
	<-done
}

func TestFeedDeliversNewIssueToProjectAndIssueTopics(t *testing.T) {
	a := apptest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "BOOK", Name: "Books"}); err != nil {
		t.Fatal(err)
	}
	last, err := a.Service.LastEventID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	issue, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "BOOK", Title: "Shelf"})
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe before feed starts, so nothing can be published unheard.
	b := bus.New()
	projCh, unsubProj := b.Subscribe("project.BOOK")
	defer unsubProj()
	issueCh, unsubIssue := b.Subscribe("issue." + issue.ID)
	defer unsubIssue()

	done := make(chan struct{})
	go func() {
		feed(ctx, a.Service, b, last, 5*time.Millisecond, discardLog())
		close(done)
	}()

	for name, ch := range map[string]<-chan []byte{"project": projCh, "issue": issueCh} {
		e := recvEvent(t, ch)
		if e.Type != core.EventIssueCreated {
			t.Errorf("%s subscriber: type = %q, want %q", name, e.Type, core.EventIssueCreated)
		}
		if e.IssueID != issue.ID {
			t.Errorf("%s subscriber: issue ID = %q, want %q", name, e.IssueID, issue.ID)
		}
	}

	cancel()
	<-done
}

func TestFeedDoesNotDeliverToOtherProject(t *testing.T) {
	a := apptest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, key := range []string{"BOOK", "OTHER"} {
		if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: key, Name: key}); err != nil {
			t.Fatal(err)
		}
	}
	last, err := a.Service.LastEventID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	b := bus.New()
	otherCh, unsubOther := b.Subscribe("project.OTHER")
	defer unsubOther()
	bookCh, unsubBook := b.Subscribe("project.BOOK")
	defer unsubBook()

	done := make(chan struct{})
	go func() {
		feed(ctx, a.Service, b, last, 5*time.Millisecond, discardLog())
		close(done)
	}()

	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "BOOK", Title: "Only BOOK"}); err != nil {
		t.Fatal(err)
	}
	// Once the BOOK subscriber has the event, feed has published everything
	// it had for this event, so OTHER must still be empty.
	recvEvent(t, bookCh)
	select {
	case data := <-otherCh:
		t.Fatalf("project.OTHER received a message for a BOOK event: %s", data)
	default:
	}

	cancel()
	<-done
}

func TestFeedReturnsOnCancel(t *testing.T) {
	a := apptest.New(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		feed(ctx, a.Service, bus.New(), 0, 5*time.Millisecond, discardLog())
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("feed did not return after its context was cancelled")
	}
}
