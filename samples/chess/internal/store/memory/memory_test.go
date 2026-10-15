package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"chess/internal/core"
)

func ctx() context.Context { return context.Background() }

func TestCreateAssignsSequentialIDs(t *testing.T) {
	s := New()
	for want := int64(1); want <= 3; want++ {
		g, err := s.Create(ctx(), core.Game{Human: core.White})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if g.ID != want {
			t.Fatalf("ID = %d, want %d", g.ID, want)
		}
	}
}

func TestGetMissingWrapsNotFound(t *testing.T) {
	s := New()
	if _, err := s.Get(ctx(), 42); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get missing: err = %v, want ErrNotFound", err)
	}
}

func TestSaveMissingWrapsNotFound(t *testing.T) {
	s := New()
	if err := s.Save(ctx(), core.Game{ID: 42}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Save missing: err = %v, want ErrNotFound", err)
	}
}

func TestListOrder(t *testing.T) {
	s := New()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// IDs 1 and 2 share an UpdatedAt; 3 is the newest; 4 is the oldest.
	games := []core.Game{
		{UpdatedAt: base},
		{UpdatedAt: base},
		{UpdatedAt: base.Add(2 * time.Hour)},
		{UpdatedAt: base.Add(-time.Hour)},
	}
	for _, g := range games {
		if _, err := s.Create(ctx(), g); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	list, err := s.List(ctx())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	wantIDs := []int64{3, 2, 1, 4}
	if len(list) != len(wantIDs) {
		t.Fatalf("len = %d, want %d", len(list), len(wantIDs))
	}
	for i, want := range wantIDs {
		if list[i].ID != want {
			t.Fatalf("list[%d].ID = %d, want %d", i, list[i].ID, want)
		}
	}
}

func TestGetReturnsCopy(t *testing.T) {
	s := New()
	if _, err := s.Create(ctx(), core.Game{Status: core.StatusActive, Legal: []string{"e2e4"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	g, err := s.Get(ctx(), 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	g.Status = core.StatusDraw
	g.Legal[0] = "changed"
	again, err := s.Get(ctx(), 1)
	if err != nil {
		t.Fatalf("Get again: %v", err)
	}
	if again.Status != core.StatusActive {
		t.Fatalf("Status changed through caller copy: %q", again.Status)
	}
	if again.Legal[0] != "e2e4" {
		t.Fatalf("Legal changed through caller copy: %q", again.Legal[0])
	}
}

func TestCreateCopiesInput(t *testing.T) {
	s := New()
	in := core.Game{Legal: []string{"e2e4"}}
	stored, err := s.Create(ctx(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	in.Legal[0] = "changed"
	stored.Legal[0] = "also changed"
	got, err := s.Get(ctx(), 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Legal[0] != "e2e4" {
		t.Fatalf("store shares slice with caller: %q", got.Legal[0])
	}
}

func TestSaveReplacesAndCopies(t *testing.T) {
	s := New()
	if _, err := s.Create(ctx(), core.Game{Status: core.StatusActive}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	g := core.Game{ID: 1, Status: core.StatusWhiteWon, Method: "checkmate"}
	if err := s.Save(ctx(), g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	g.Method = "changed after save"
	got, err := s.Get(ctx(), 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != core.StatusWhiteWon || got.Method != "checkmate" {
		t.Fatalf("Save did not replace, or shares state with caller: %+v", got)
	}
}

func TestMovesWithTurnSurviveRoundTrip(t *testing.T) {
	s := New()
	turn := &core.Turn{
		Route:      "local",
		Backend:    "llama",
		Model:      "small",
		Policy:     map[string]any{"temperature": 0.5},
		Candidates: []string{"Nf3", "e4"},
		Scores:     map[string]float64{"g1f3": 0.7},
		Attempts:   2,
		Fallback:   "random",
		Errors:     []string{"bad move"},
	}
	in := core.Game{
		Moves: []core.Move{
			{UCI: "e2e4", SAN: "e4", By: core.White},
			{UCI: "e7e5", SAN: "e5", By: core.Black, Turn: turn},
		},
	}
	created, err := s.Create(ctx(), in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.Get(ctx(), created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Moves) != 2 {
		t.Fatalf("len(Moves) = %d, want 2", len(got.Moves))
	}
	if got.Moves[0].Turn != nil {
		t.Fatalf("Moves[0].Turn = %+v, want nil", got.Moves[0].Turn)
	}
	gt := got.Moves[1].Turn
	if gt == nil {
		t.Fatal("Moves[1].Turn is nil after round trip")
	}
	if gt.Route != "local" || gt.Backend != "llama" || gt.Model != "small" ||
		gt.Attempts != 2 || gt.Fallback != "random" {
		t.Fatalf("Turn scalars lost: %+v", gt)
	}
	if len(gt.Candidates) != 2 || gt.Candidates[0] != "Nf3" {
		t.Fatalf("Turn.Candidates = %v", gt.Candidates)
	}
	if gt.Scores["g1f3"] != 0.7 {
		t.Fatalf("Turn.Scores = %v", gt.Scores)
	}
	if gt.Policy["temperature"] != 0.5 {
		t.Fatalf("Turn.Policy = %v", gt.Policy)
	}
	if len(gt.Errors) != 1 || gt.Errors[0] != "bad move" {
		t.Fatalf("Turn.Errors = %v", gt.Errors)
	}
	// The stored Turn must not share the pointer with the caller's value.
	turn.Attempts = 99
	again, err := s.Get(ctx(), created.ID)
	if err != nil {
		t.Fatalf("Get again: %v", err)
	}
	if again.Moves[1].Turn.Attempts != 2 {
		t.Fatalf("store shares Turn pointer with caller")
	}
}
