package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"chess/internal/core"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chess.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func at(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

func newGame(sec int64) core.Game {
	return core.Game{
		Human:      core.White,
		Difficulty: "normal",
		Status:     core.StatusActive,
		CreatedAt:  at(sec),
		UpdatedAt:  at(sec),
	}
}

func TestCreateAssignsSequentialIDs(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	for want := int64(1); want <= 3; want++ {
		g, err := s.Create(ctx, newGame(want))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if g.ID != want {
			t.Fatalf("ID = %d, want %d", g.ID, want)
		}
	}
}

func TestGetMissingWrapsNotFound(t *testing.T) {
	s, _ := openTemp(t)
	_, err := s.Get(context.Background(), 42)
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get missing: err = %v, want ErrNotFound", err)
	}
}

func TestSaveMissingWrapsNotFound(t *testing.T) {
	s, _ := openTemp(t)
	g := newGame(1)
	g.ID = 42
	err := s.Save(context.Background(), g)
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Save missing: err = %v, want ErrNotFound", err)
	}
}

func TestListOrder(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	// IDs 1 and 2 share an UpdatedAt; the higher ID must come first.
	for _, sec := range []int64{100, 100, 50} {
		if _, err := s.Create(ctx, newGame(sec)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	// Move game 1 to the newest time.
	g, err := s.Get(ctx, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	g.UpdatedAt = at(200)
	if err := s.Save(ctx, g); err != nil {
		t.Fatalf("Save: %v", err)
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got []int64
	for _, g := range list {
		got = append(got, g.ID)
	}
	want := []int64{1, 2, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List IDs = %v, want %v", got, want)
	}
}

func TestListEmptyIsNotNil(t *testing.T) {
	s, _ := openTemp(t)
	list, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil {
		t.Fatal("List returned nil, want empty slice")
	}
}

func TestMovesWithTurnSurviveRoundTrip(t *testing.T) {
	s, _ := openTemp(t)
	ctx := context.Background()
	g := newGame(10)
	g.Moves = []core.Move{
		{UCI: "e2e4", SAN: "e4", By: core.White},
		{
			UCI: "e7e5", SAN: "e5", By: core.Black,
			Turn: &core.Turn{
				Route:      "llm",
				Backend:    "router",
				Model:      "m1",
				Policy:     map[string]any{"temperature": 0.5},
				Candidates: []string{"e5", "c5"},
				Scores:     map[string]float64{"e7e5": 0.9},
				Attempts:   2,
				Fallback:   "",
				Errors:     []string{"first try illegal"},
			},
		},
	}
	g.Status = core.StatusDraw
	g.Method = "stalemate"

	created, err := s.Create(ctx, g)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	created.UpdatedAt = at(20)
	if err := s.Save(ctx, created); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got.Moves, g.Moves) {
		t.Fatalf("Moves = %+v, want %+v", got.Moves, g.Moves)
	}
	if got.Moves[1].Turn == nil || got.Moves[1].Turn.Attempts != 2 {
		t.Fatalf("Turn lost in round trip: %+v", got.Moves[1].Turn)
	}
	if got.Status != core.StatusDraw || got.Method != "stalemate" {
		t.Fatalf("Status/Method = %q/%q", got.Status, got.Method)
	}
	if got.ID != created.ID {
		t.Fatalf("ID = %d, want %d", got.ID, created.ID)
	}
	if !got.UpdatedAt.Equal(at(20)) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, at(20))
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	s, path := openTemp(t)
	ctx := context.Background()
	g, err := s.Create(ctx, newGame(5))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	g.Moves = []core.Move{{UCI: "d2d4", SAN: "d4", By: core.White}}
	if err := s.Save(ctx, g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Get(ctx, g.ID)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if len(got.Moves) != 1 || got.Moves[0].UCI != "d2d4" {
		t.Fatalf("Moves after reopen = %+v", got.Moves)
	}
	next, err := s2.Create(ctx, newGame(6))
	if err != nil {
		t.Fatalf("Create after reopen: %v", err)
	}
	if next.ID != 2 {
		t.Fatalf("ID after reopen = %d, want 2", next.ID)
	}
}
