package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chess/internal/api"
	"chess/internal/apptest"
	"chess/internal/core"
)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(api.NewHandler(apptest.New(t)))
	t.Cleanup(srv.Close)
	return srv
}

// do sends a request and returns the status and the body.
func do(t *testing.T, srv *httptest.Server, method, path, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("%s %s: content type %q, want application/json", method, path, ct)
	}
	return resp.StatusCode, data
}

func decodeGame(t *testing.T, data []byte) core.Game {
	t.Helper()
	var g core.Game
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("decode game: %v: %s", err, data)
	}
	return g
}

func expectStatus(t *testing.T, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status %d, want %d: %s", got, want, body)
	}
}

func TestCreateListAndGet(t *testing.T) {
	srv := newServer(t)

	code, body := do(t, srv, "POST", "/api/games", `{"human":"white","difficulty":"easy"}`)
	expectStatus(t, code, http.StatusCreated, body)
	created := decodeGame(t, body)
	if created.Human != core.White || created.Difficulty != "easy" || created.Turn != core.White {
		t.Errorf("created game = %+v", created)
	}
	if len(created.Legal) == 0 {
		t.Error("created game has no legal moves")
	}

	code, body = do(t, srv, "GET", "/api/games", "")
	expectStatus(t, code, http.StatusOK, body)
	var list []core.Game
	if err := json.Unmarshal(body, &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %s, err %v; want one game", body, err)
	}

	code, body = do(t, srv, "GET", "/api/games/1", "")
	expectStatus(t, code, http.StatusOK, body)
	if got := decodeGame(t, body); got.ID != created.ID {
		t.Errorf("get id = %d, want %d", got.ID, created.ID)
	}
}

func TestEmptyListIsArray(t *testing.T) {
	srv := newServer(t)
	code, body := do(t, srv, "GET", "/api/games", "")
	expectStatus(t, code, http.StatusOK, body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("empty list = %s, want []", body)
	}
}

func TestCreateAsBlackOpponentMovesFirst(t *testing.T) {
	srv := newServer(t)
	code, body := do(t, srv, "POST", "/api/games", `{"human":"black"}`)
	expectStatus(t, code, http.StatusCreated, body)
	g := decodeGame(t, body)
	if len(g.Moves) != 1 || g.Moves[0].By != core.White {
		t.Errorf("moves = %+v, want one white move", g.Moves)
	}
	if g.Difficulty != "normal" {
		t.Errorf("difficulty = %q, want normal", g.Difficulty)
	}
}

func TestPlayE4(t *testing.T) {
	srv := newServer(t)
	do(t, srv, "POST", "/api/games", `{"human":"white"}`)

	code, body := do(t, srv, "POST", "/api/games/1/moves", `{"move":"e4"}`)
	expectStatus(t, code, http.StatusOK, body)
	g := decodeGame(t, body)
	if len(g.Moves) != 2 {
		t.Fatalf("moves = %+v, want two", g.Moves)
	}
	if g.Moves[0].UCI != "e2e4" || g.Moves[0].SAN != "e4" || g.Moves[0].By != core.White {
		t.Errorf("human move = %+v", g.Moves[0])
	}
	if g.Moves[1].By != core.Black {
		t.Errorf("opponent move = %+v, want black", g.Moves[1])
	}
	if g.Turn != core.White {
		t.Errorf("turn = %q, want white", g.Turn)
	}
}

func TestIllegalMoveIsBadRequest(t *testing.T) {
	srv := newServer(t)
	do(t, srv, "POST", "/api/games", `{"human":"white"}`)
	code, body := do(t, srv, "POST", "/api/games/1/moves", `{"move":"e5"}`)
	expectStatus(t, code, http.StatusBadRequest, body)
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.Error == "" {
		t.Errorf("error body = %s, err %v", body, err)
	}
}

func TestUnknownGameIsNotFound(t *testing.T) {
	srv := newServer(t)
	code, body := do(t, srv, "GET", "/api/games/999", "")
	expectStatus(t, code, http.StatusNotFound, body)
	code, body = do(t, srv, "POST", "/api/games/999/moves", `{"move":"e4"}`)
	expectStatus(t, code, http.StatusNotFound, body)
	code, body = do(t, srv, "POST", "/api/games/999/resign", "")
	expectStatus(t, code, http.StatusNotFound, body)
}

func TestBadRequests(t *testing.T) {
	srv := newServer(t)
	cases := []struct {
		name, method, path, body string
	}{
		{"invalid human", "POST", "/api/games", `{"human":"green"}`},
		{"malformed create body", "POST", "/api/games", `not json`},
		{"non-integer id", "GET", "/api/games/abc", ""},
		{"malformed move body", "POST", "/api/games/1/moves", `{"move":`},
	}
	do(t, srv, "POST", "/api/games", `{"human":"white"}`)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := do(t, srv, c.method, c.path, c.body)
			expectStatus(t, code, http.StatusBadRequest, body)
		})
	}
}

func TestMoveAfterResignIsConflict(t *testing.T) {
	srv := newServer(t)
	do(t, srv, "POST", "/api/games", `{"human":"white"}`)

	code, body := do(t, srv, "POST", "/api/games/1/resign", "")
	expectStatus(t, code, http.StatusOK, body)
	g := decodeGame(t, body)
	if g.Status != core.StatusBlackWon || g.Method != "resignation" {
		t.Errorf("resigned game status = %q method %q", g.Status, g.Method)
	}

	code, body = do(t, srv, "POST", "/api/games/1/moves", `{"move":"e4"}`)
	expectStatus(t, code, http.StatusConflict, body)

	code, body = do(t, srv, "POST", "/api/games/1/resign", "")
	expectStatus(t, code, http.StatusConflict, body)
}

func TestHealthz(t *testing.T) {
	srv := newServer(t)
	code, body := do(t, srv, "GET", "/healthz", "")
	expectStatus(t, code, http.StatusOK, body)
	var h map[string]string
	if err := json.Unmarshal(body, &h); err != nil || h["status"] != "ok" {
		t.Errorf("healthz = %s, err %v", body, err)
	}
}
