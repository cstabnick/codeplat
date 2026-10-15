package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sandbox/internal/apptest"
	"sandbox/internal/core"
	"sandbox/internal/httpapi"
)

func newHandler(t *testing.T) http.Handler {
	t.Helper()
	return httpapi.NewHandler(apptest.New(t))
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, want, rec.Body.String())
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}

func TestPutThenExec(t *testing.T) {
	h := newHandler(t)

	rec := do(t, h, http.MethodPut, "/api/boxes/demo", `{}`)
	wantStatus(t, rec, http.StatusOK)
	var box core.Box
	decode(t, rec, &box)
	if box.Name != "demo" {
		t.Errorf("box name = %q, want demo", box.Name)
	}

	rec = do(t, h, http.MethodPost, "/api/boxes/demo/exec", `{"command":"echo hi","timeout_seconds":60}`)
	wantStatus(t, rec, http.StatusOK)
	var res core.ExecResult
	decode(t, rec, &res)
	if res.Output != "hi\n" {
		t.Errorf("output = %q, want %q", res.Output, "hi\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0", res.ExitCode)
	}
	if res.TimedOut {
		t.Errorf("timed_out = true, want false")
	}
}

func TestPutTwiceIsFine(t *testing.T) {
	h := newHandler(t)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/demo", ``), http.StatusOK)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/demo", `{}`), http.StatusOK)
}

func TestBadNameIsBadRequest(t *testing.T) {
	h := newHandler(t)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/Bad_Name", `{}`), http.StatusBadRequest)
}

func TestMalformedBodyIsBadRequest(t *testing.T) {
	h := newHandler(t)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/demo", `{"workspace":`), http.StatusBadRequest)
}

func TestExecUnknownBoxIsNotFound(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, http.MethodPost, "/api/boxes/missing/exec", `{"command":"echo hi"}`)
	wantStatus(t, rec, http.StatusNotFound)
	var body map[string]string
	decode(t, rec, &body)
	if body["error"] == "" {
		t.Errorf("error body missing message: %s", rec.Body.String())
	}
}

func TestExecEmptyCommandIsBadRequest(t *testing.T) {
	h := newHandler(t)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/demo", `{}`), http.StatusOK)
	wantStatus(t, do(t, h, http.MethodPost, "/api/boxes/demo/exec", `{"command":"  "}`), http.StatusBadRequest)
}

func TestListReturnsBoxes(t *testing.T) {
	h := newHandler(t)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/demo", `{}`), http.StatusOK)

	rec := do(t, h, http.MethodGet, "/api/boxes", "")
	wantStatus(t, rec, http.StatusOK)
	var boxes []core.Box
	decode(t, rec, &boxes)
	if len(boxes) != 1 || boxes[0].Name != "demo" {
		t.Errorf("list = %+v, want one box named demo", boxes)
	}
}

func TestListEmptyIsArrayNotNull(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, http.MethodGet, "/api/boxes", "")
	wantStatus(t, rec, http.StatusOK)
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("empty list body = %q, want []", got)
	}
}

func TestDeleteThenGetIsNotFound(t *testing.T) {
	h := newHandler(t)
	wantStatus(t, do(t, h, http.MethodPut, "/api/boxes/demo", `{}`), http.StatusOK)
	wantStatus(t, do(t, h, http.MethodDelete, "/api/boxes/demo", ""), http.StatusNoContent)
	wantStatus(t, do(t, h, http.MethodGet, "/api/boxes/demo", ""), http.StatusNotFound)
}

func TestHealthz(t *testing.T) {
	h := newHandler(t)
	rec := do(t, h, http.MethodGet, "/healthz", "")
	wantStatus(t, rec, http.StatusOK)
	var body map[string]string
	decode(t, rec, &body)
	if body["status"] != "ok" {
		t.Errorf("healthz = %v, want status ok", body)
	}
}
