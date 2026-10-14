package sse

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const timeout = 5 * time.Second

// fakeSub is a Subscriber that records the topics it was asked for and
// delivers whatever the test sends on ch.
type fakeSub struct {
	ch        chan []byte
	cancelled chan struct{}
	once      sync.Once

	mu     sync.Mutex
	topics []string
}

func newFakeSub() *fakeSub {
	return &fakeSub{ch: make(chan []byte, 10), cancelled: make(chan struct{})}
}

func (f *fakeSub) Subscribe(topic string) (<-chan []byte, func()) {
	f.mu.Lock()
	f.topics = append(f.topics, topic)
	f.mu.Unlock()
	return f.ch, func() {
		f.once.Do(func() { close(f.cancelled) })
	}
}

func (f *fakeSub) subscribedTopics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.topics...)
}

// stream reads an SSE response line by line in the background so that tests
// can wait for a block with a timeout instead of blocking forever.
type stream struct {
	t     *testing.T
	lines chan string
}

func newStream(t *testing.T, ctx context.Context, body io.Reader) *stream {
	s := &stream{t: t, lines: make(chan string, 64)}
	go func() {
		defer close(s.lines)
		br := bufio.NewReader(body)
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			select {
			case s.lines <- line:
			case <-ctx.Done():
				return
			}
		}
	}()
	return s
}

// block returns the next block of lines, up to and including the blank line
// that ends it, so an event's bytes can be compared exactly.
func (s *stream) block() string {
	s.t.Helper()
	var b strings.Builder
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-s.lines:
			if !ok {
				s.t.Fatalf("stream ended after %q", b.String())
			}
			b.WriteString(line)
			if line == "\n" {
				return b.String()
			}
		case <-deadline:
			s.t.Fatalf("timed out waiting for a block; read so far %q", b.String())
		}
	}
}

// connect opens a stream to the handler at srv with the given raw query. The
// request is cancelled when the test ends, before the server is closed.
func connect(t *testing.T, srv *httptest.Server, query string) (*http.Response, *stream, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/?"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp, newStream(t, ctx, resp.Body), cancel
}

func newServer(t *testing.T, sub Subscriber) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(Handler(sub))
	t.Cleanup(srv.Close)
	return srv
}

func TestMissingTopicIsBadRequest(t *testing.T) {
	sub := newFakeSub()
	srv := newServer(t, sub)

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if got := strings.TrimSpace(string(body)); got != "topic is required" {
		t.Errorf("body = %q, want %q", got, "topic is required")
	}
	if topics := sub.subscribedTopics(); len(topics) != 0 {
		t.Errorf("Subscribe was called for %v on a request without a topic", topics)
	}
}

func TestNonFlusherWriterIsInternalError(t *testing.T) {
	sub := newFakeSub()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?topic=board", nil)

	// Hide Flush by embedding only the interface.
	Handler(sub).ServeHTTP(struct{ http.ResponseWriter }{rec}, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "streaming unsupported" {
		t.Errorf("body = %q, want %q", got, "streaming unsupported")
	}
	if topics := sub.subscribedTopics(); len(topics) != 0 {
		t.Errorf("Subscribe was called for %v on a non-flusher writer", topics)
	}
}

func TestStreamHeadersAndConnectedLine(t *testing.T) {
	sub := newFakeSub()
	srv := newServer(t, sub)

	resp, st, _ := connect(t, srv, "topic=board")

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", got, "text/event-stream")
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
	if got := st.block(); got != ": connected\n\n" {
		t.Errorf("first block = %q, want %q", got, ": connected\n\n")
	}
}

func TestSubscribesToExactlyTheQueryTopic(t *testing.T) {
	sub := newFakeSub()
	srv := newServer(t, sub)

	topic := "a b&c"
	_, st, _ := connect(t, srv, "topic="+url.QueryEscape(topic))
	// The connected line is written after Subscribe, so it is a safe point
	// to inspect the topics.
	st.block()

	got := sub.subscribedTopics()
	if len(got) != 1 || got[0] != topic {
		t.Errorf("Subscribe topics = %q, want [%q]", got, topic)
	}
}

func TestMessageArrivesAsEvent(t *testing.T) {
	sub := newFakeSub()
	srv := newServer(t, sub)
	_, st, _ := connect(t, srv, "topic=board")
	st.block() // connected

	sub.ch <- []byte(`{"id":3}`)

	want := "event: changed\ndata: {\"id\":3}\n\n"
	if got := st.block(); got != want {
		t.Errorf("event = %q, want %q", got, want)
	}
}

func TestMultilineMessageBecomesSeveralDataLines(t *testing.T) {
	sub := newFakeSub()
	srv := newServer(t, sub)
	_, st, _ := connect(t, srv, "topic=board")
	st.block() // connected

	sub.ch <- []byte("first\nsecond")

	want := "event: changed\ndata: first\ndata: second\n\n"
	if got := st.block(); got != want {
		t.Errorf("event = %q, want %q", got, want)
	}
}

func TestPingIsSentOnIdleStream(t *testing.T) {
	old := keepAlive
	keepAlive = 5 * time.Millisecond
	t.Cleanup(func() { keepAlive = old })

	sub := newFakeSub()
	srv := newServer(t, sub)
	_, st, _ := connect(t, srv, "topic=board")

	if got := st.block(); got != ": connected\n\n" {
		t.Fatalf("first block = %q, want the connected line", got)
	}
	if got := st.block(); got != ": ping\n\n" {
		t.Errorf("idle block = %q, want %q", got, ": ping\n\n")
	}
}

func TestDisconnectCallsCancel(t *testing.T) {
	sub := newFakeSub()
	srv := newServer(t, sub)
	_, st, disconnect := connect(t, srv, "topic=board")
	st.block() // connected, so Subscribe has run

	disconnect()

	select {
	case <-sub.cancelled:
	case <-time.After(timeout):
		t.Fatal("cancel was not called after the client disconnected")
	}
}
