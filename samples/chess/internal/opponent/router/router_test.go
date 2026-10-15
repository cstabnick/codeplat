package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"chess/internal/core"
)

func testRequest() core.MoveRequest {
	return core.MoveRequest{
		FEN:         "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
		Color:       core.White,
		History:     nil,
		Legal:       []string{"Nf3", "e4"},
		Count:       2,
		Temperature: 0.7,
		Persona:     "You are a patient coach.",
	}
}

// capture serves reply on every request and records the request body and headers.
type capture struct {
	body    map[string]any
	headers http.Header
	path    string
	status  int
	reply   string
}

func (c *capture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.path = r.URL.Path
	c.headers = r.Header.Clone()
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &c.body)
	for k, v := range map[string]string{
		"X-Llmrouter-Route":   "fast",
		"X-Llmrouter-Backend": "bonsai",
		"X-Llmrouter-Model":   "bonsai-8b",
	} {
		w.Header().Set(k, v)
	}
	if c.status != 0 {
		w.WriteHeader(c.status)
	}
	_, _ = io.WriteString(w, c.reply)
}

func chatReply(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"role": "assistant", "content": content}}},
	})
	return string(b)
}

func TestProposeRequestBodyAndModelChoice(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		route      string
		want       string
	}{
		{"route wins over configured", "cfg-model", "route-model", "route-model"},
		{"configured used without route", "cfg-model", "", "cfg-model"},
		{"auto when neither set", "", "", "auto"},
		{"quick route", "cfg-model", "quick", "quick"},
		{"general route", "cfg-model", "general", "general"},
		{"reason route", "cfg-model", "reason", "reason"},
		{"empty route uses configured", "cfg-model", "", "cfg-model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cap := &capture{reply: chatReply(`{"moves": ["Nf3"]}`)}
			srv := httptest.NewServer(cap)
			defer srv.Close()

			c := New(srv.URL+"/", tc.configured, nil)
			req := testRequest()
			req.Route = tc.route
			if _, err := c.Propose(context.Background(), req); err != nil {
				t.Fatalf("Propose: %v", err)
			}

			if cap.path != "/v1/chat/completions" {
				t.Errorf("path = %q", cap.path)
			}
			if got := cap.body["model"]; got != tc.want {
				t.Errorf("model = %v, want %q", got, tc.want)
			}
			kwargs, _ := cap.body["chat_template_kwargs"].(map[string]any)
			if got := kwargs["enable_thinking"]; got != false {
				t.Errorf("chat_template_kwargs = %v, want enable_thinking false", cap.body["chat_template_kwargs"])
			}
			if got := cap.body["stream"]; got != false {
				t.Errorf("stream = %v, want false", got)
			}
			if got := cap.body["temperature"]; got != 0.7 {
				t.Errorf("temperature = %v, want 0.7", got)
			}

			msgs, _ := cap.body["messages"].([]any)
			if len(msgs) != 2 {
				t.Fatalf("messages = %v, want system and user", cap.body["messages"])
			}
			sys := msgs[0].(map[string]any)
			user := msgs[1].(map[string]any)
			if sys["role"] != "system" || user["role"] != "user" {
				t.Errorf("roles = %v, %v", sys["role"], user["role"])
			}
			sysText := sys["content"].(string)
			for _, want := range []string{"You are a patient coach.", "You play white", "up to 2 moves in SAN"} {
				if !strings.Contains(sysText, want) {
					t.Errorf("system prompt %q missing %q", sysText, want)
				}
			}
			userText := user["content"].(string)
			for _, want := range []string{
				"Position (FEN): rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
				"Moves so far: none",
				"Legal moves: Nf3 e4",
			} {
				if !strings.Contains(userText, want) {
					t.Errorf("user prompt %q missing %q", userText, want)
				}
			}
			if strings.Contains(userText, "Feedback") {
				t.Errorf("user prompt has feedback without any: %q", userText)
			}
		})
	}
}

func TestProposeIncludesHistoryAndFeedback(t *testing.T) {
	cap := &capture{reply: chatReply(`{"moves": ["e4"]}`)}
	srv := httptest.NewServer(cap)
	defer srv.Close()

	req := testRequest()
	req.History = []string{"d4", "d5"}
	req.Feedback = "Nf6 is not legal here"
	if _, err := New(srv.URL, "", nil).Propose(context.Background(), req); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	userText := cap.body["messages"].([]any)[1].(map[string]any)["content"].(string)
	if !strings.Contains(userText, "Moves so far: d4 d5") {
		t.Errorf("user prompt %q missing history", userText)
	}
	if !strings.Contains(userText, "Nf6 is not legal here") {
		t.Errorf("user prompt %q missing feedback", userText)
	}
}

func TestProposeJSONReplyAndHeaders(t *testing.T) {
	cap := &capture{reply: chatReply(`Here you go: {"moves": ["Nf3", "e4", "Nf3", "Bc4"]}`)}
	srv := httptest.NewServer(cap)
	defer srv.Close()

	req := testRequest()
	req.Count = 2
	reply, err := New(srv.URL, "", nil).Propose(context.Background(), req)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if want := []string{"Nf3", "e4"}; !reflect.DeepEqual(reply.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", reply.Candidates, want)
	}
	if reply.Route != "fast" || reply.Backend != "bonsai" || reply.Model != "bonsai-8b" {
		t.Errorf("headers not copied: route=%q backend=%q model=%q", reply.Route, reply.Backend, reply.Model)
	}
	if !strings.Contains(reply.Raw, `"moves"`) {
		t.Errorf("Raw = %q", reply.Raw)
	}
}

func TestProposeProseReply(t *testing.T) {
	cap := &capture{reply: chatReply("I think Nf3 is best, then e4 or maybe Bb5+. Also Nf3 again.")}
	srv := httptest.NewServer(cap)
	defer srv.Close()

	req := testRequest()
	req.Count = 3
	reply, err := New(srv.URL, "", nil).Propose(context.Background(), req)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if want := []string{"Nf3", "e4", "Bb5+"}; !reflect.DeepEqual(reply.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", reply.Candidates, want)
	}
}

func TestProposeCapsCandidatesAtCount(t *testing.T) {
	cap := &capture{reply: chatReply(`{"moves": ["Nf3", "e4", "d4", "c4"]}`)}
	srv := httptest.NewServer(cap)
	defer srv.Close()

	req := testRequest()
	req.Count = 1
	reply, err := New(srv.URL, "", nil).Propose(context.Background(), req)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if want := []string{"Nf3"}; !reflect.DeepEqual(reply.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", reply.Candidates, want)
	}
}

func TestProposeHTTPError(t *testing.T) {
	long := strings.Repeat("x", 600)
	cap := &capture{status: http.StatusInternalServerError, reply: long}
	srv := httptest.NewServer(cap)
	defer srv.Close()

	_, err := New(srv.URL, "", nil).Propose(context.Background(), testRequest())
	if err == nil {
		t.Fatal("Propose: want error for 500")
	}
	msg := err.Error()
	if !strings.Contains(msg, "500") {
		t.Errorf("error %q does not mention the status", msg)
	}
	if strings.Count(msg, "x") > 512 {
		t.Errorf("error includes more than 512 bytes of body (%d x's)", strings.Count(msg, "x"))
	}
	if strings.Count(msg, "x") == 0 {
		t.Errorf("error %q does not include the body", msg)
	}
}

func TestProposeUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := New(url, "", nil).Propose(context.Background(), testRequest())
	if err == nil {
		t.Fatal("Propose: want error when router is down")
	}
	if !strings.HasPrefix(err.Error(), "router unreachable: ") {
		t.Errorf("error = %q, want prefix %q", err, "router unreachable: ")
	}
}

func TestProposeCanceledContextReturnsContextError(t *testing.T) {
	srv := httptest.NewServer(&capture{reply: chatReply(`{"moves": ["Nf3"]}`)})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(srv.URL, "", nil).Propose(ctx, testRequest())
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
