// Package router implements core.Mover by asking a model through the LLM router.
package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"chess/internal/core"
)

// Client asks the LLM router for chess moves.
type Client struct {
	base  string
	model string
	hc    *http.Client
}

var _ core.Mover = (*Client)(nil)

// New returns a Client for the router at baseURL. An empty model means "auto".
// A nil hc means a default http.Client.
func New(baseURL, model string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), model: model, hc: hc}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model              string         `json:"model"`
	Stream             bool           `json:"stream"`
	Temperature        float64        `json:"temperature"`
	Messages           []message      `json:"messages"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs"`
}

// noThinking turns off model reasoning for move requests. A move is a short
// JSON reply, and reasoning first makes the router take minutes.
var noThinking = map[string]any{"enable_thinking": false}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Propose sends the position to the router and returns the candidate moves it names.
func (c *Client) Propose(ctx context.Context, req core.MoveRequest) (core.MoveReply, error) {
	model := req.Route
	if model == "" {
		model = c.model
	}
	if model == "" {
		model = "auto"
	}
	body, err := json.Marshal(chatRequest{
		Model:       model,
		Stream:      false,
		Temperature: req.Temperature,
		Messages: []message{
			{Role: "system", Content: systemPrompt(req)},
			{Role: "user", Content: userPrompt(req)},
		},
		ChatTemplateKwargs: noThinking,
	})
	if err != nil {
		return core.MoveReply{}, fmt.Errorf("router: encode request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return core.MoveReply{}, fmt.Errorf("router: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return core.MoveReply{}, ctxErr
		}
		return core.MoveReply{}, fmt.Errorf("router unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return core.MoveReply{}, fmt.Errorf("router: status %d: %s", resp.StatusCode, snippet)
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return core.MoveReply{}, fmt.Errorf("router: decode reply: %w", err)
	}
	if len(out.Choices) == 0 {
		return core.MoveReply{}, fmt.Errorf("router: reply has no choices")
	}
	raw := out.Choices[0].Message.Content
	return core.MoveReply{
		Candidates: candidates(raw, req.Count),
		Route:      resp.Header.Get("X-Llmrouter-Route"),
		Backend:    resp.Header.Get("X-Llmrouter-Backend"),
		Model:      resp.Header.Get("X-Llmrouter-Model"),
		Raw:        raw,
	}, nil
}

func systemPrompt(req core.MoveRequest) string {
	prefix := strings.TrimSpace(req.Persona)
	if prefix != "" {
		prefix += " "
	}
	return fmt.Sprintf(`%sYou play %s. Reply with only JSON: {"moves": ["<move>", ...]} listing up to %d moves in SAN, best first.`,
		prefix, req.Color, req.Count)
}

func userPrompt(req core.MoveRequest) string {
	history := "none"
	if len(req.History) > 0 {
		history = strings.Join(req.History, " ")
	}
	lines := []string{
		"Position (FEN): " + req.FEN,
		"Moves so far: " + history,
		"Legal moves: " + strings.Join(req.Legal, " "),
	}
	if req.Feedback != "" {
		lines = append(lines, "Feedback: "+req.Feedback)
	}
	return strings.Join(lines, "\n")
}

// sanToken matches one move in SAN, optionally followed by check or mate marks.
var sanToken = regexp.MustCompile(`^(O-O(-O)?|[KQRBN]?[a-h]?[1-8]?x?[a-h][1-8](=[QRBN])?|[a-h][1-8][a-h][1-8][qrbn]?)[+#]?$`)

// candidates reads moves from a model reply. It prefers the JSON object between
// the first '{' and the last '}'; otherwise it takes every SAN-shaped token in
// order. The result is unique and holds at most count moves.
func candidates(raw string, count int) []string {
	var moves []string
	parsed := false
	if start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); start >= 0 && end > start {
		var obj struct {
			Moves []string `json:"moves"`
		}
		if json.Unmarshal([]byte(raw[start:end+1]), &obj) == nil {
			moves = obj.Moves
			parsed = true
		}
	}
	if !parsed {
		for _, tok := range strings.Fields(raw) {
			tok = strings.Trim(tok, ".,;:!?()[]{}\"'`")
			if sanToken.MatchString(tok) {
				moves = append(moves, tok)
			}
		}
	}

	seen := make(map[string]bool, len(moves))
	out := []string{}
	for _, m := range moves {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			continue
		}
		if len(out) >= count {
			break
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}
