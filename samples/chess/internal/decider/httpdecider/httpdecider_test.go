package httpdecider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"chess/internal/core"
)

func TestDecideSendsPathAndBodyAndReturnsResult(t *testing.T) {
	var gotPath, gotMethod, gotType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"result":{"move":"e4","score":12}}`)
	}))
	defer srv.Close()

	c := New(srv.URL+"/", nil)
	out, err := c.Decide(context.Background(), core.DecisionPolicy, map[string]any{"fen": "start"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/evaluate/"+core.DecisionPolicy {
		t.Errorf("path = %q, want /api/evaluate/%s", gotPath, core.DecisionPolicy)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotType)
	}
	ctx, ok := gotBody["context"].(map[string]any)
	if !ok || ctx["fen"] != "start" {
		t.Errorf("body = %v, want {\"context\": {\"fen\": \"start\"}}", gotBody)
	}
	if out["move"] != "e4" || out["score"] != float64(12) {
		t.Errorf("result = %v, want move e4 and score 12", out)
	}
}

func TestDecideEscapesKey(t *testing.T) {
	var gotEscaped string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEscaped = r.URL.EscapedPath()
		io.WriteString(w, `{"result":{}}`)
	}))
	defer srv.Close()

	if _, err := New(srv.URL, nil).Decide(context.Background(), "a b/c", nil); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if want := "/api/evaluate/a%20b%2Fc"; gotEscaped != want {
		t.Errorf("escaped path = %q, want %q", gotEscaped, want)
	}
}

func TestDecideNullResultBecomesEmptyMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result":null}`)
	}))
	defer srv.Close()

	out, err := New(srv.URL, nil).Decide(context.Background(), "k", nil)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("result = %#v, want empty non-nil map", out)
	}
}

func TestDecideUnknownModelReturnsStatusAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"unknown model: nope"}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, nil).Decide(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("Decide: want error for 404")
	}
	want := `decide returned 404: {"error":"unknown model: nope"}`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestDecideErrorBodyIsTruncated(t *testing.T) {
	long := strings.Repeat("x", 2000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, long)
	}))
	defer srv.Close()

	_, err := New(srv.URL, nil).Decide(context.Background(), "k", nil)
	if err == nil {
		t.Fatal("Decide: want error for 400")
	}
	want := "decide returned 400: " + strings.Repeat("x", 512)
	if err.Error() != want {
		t.Errorf("error length = %d, want %d (512 byte body)", len(err.Error()), len(want))
	}
}

func TestDecideUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()

	_, err := New(addr, nil).Decide(context.Background(), "k", nil)
	if err == nil {
		t.Fatal("Decide: want error for unreachable server")
	}
	if !strings.HasPrefix(err.Error(), "decide unreachable: ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "decide unreachable: ")
	}
}
