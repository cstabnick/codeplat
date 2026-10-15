package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"decide/internal/apptest"
	"decide/internal/httpapi"
)

func do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := httpapi.NewHandler(apptest.New(t))
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return m
}

func TestEvaluate(t *testing.T) {
	rec := do(t, http.MethodPost, "/api/evaluate/example-shipping", `{"context":{"country":"US","weight":3}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := map[string]any{"result": map[string]any{"cost": float64(5)}}
	if got := decode(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}

func TestEvaluateUnknownKey(t *testing.T) {
	rec := do(t, http.MethodPost, "/api/evaluate/nope", `{"context":{}}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body %s", rec.Code, rec.Body)
	}
	if _, ok := decode(t, rec)["error"]; !ok {
		t.Errorf("body %s has no error field", rec.Body)
	}
}

func TestEvaluateBadRequests(t *testing.T) {
	cases := map[string]string{
		"bad JSON":           `{"context":`,
		"context not object": `{"context":5}`,
		"body over 1 MiB":    `{"context":{"x":"` + strings.Repeat("a", 1<<20) + `"}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, http.MethodPost, "/api/evaluate/example-shipping", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body %.80s", rec.Code, rec.Body)
			}
			if _, ok := decode(t, rec)["error"]; !ok {
				t.Errorf("body has no error field")
			}
		})
	}
}

func TestModels(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/models", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	models, ok := decode(t, rec)["models"].([]any)
	if !ok {
		t.Fatalf("body %s has no models list", rec.Body)
	}
	found := false
	for _, m := range models {
		if m == "example-shipping" {
			found = true
		}
	}
	if !found {
		t.Errorf("models %v does not include example-shipping", models)
	}
}

func TestHealthz(t *testing.T) {
	rec := do(t, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	want := map[string]any{"status": "ok"}
	if got := decode(t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %v, want %v", got, want)
	}
}
