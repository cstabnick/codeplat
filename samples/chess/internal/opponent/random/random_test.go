package random

import (
	"context"
	"reflect"
	"testing"

	"chess/internal/core"
)

func legalMoves() []string {
	return []string{"Nf3", "e4", "d4", "c4", "Nc3", "g3", "b3", "a3", "h3", "Na3"}
}

func propose(t *testing.T, m *Mover, count int, legal []string) core.MoveReply {
	t.Helper()
	reply, err := m.Propose(context.Background(), core.MoveRequest{Legal: legal, Count: count})
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	return reply
}

func TestSameSeedSameMoves(t *testing.T) {
	a := propose(t, New(42), 5, legalMoves())
	b := propose(t, New(42), 5, legalMoves())
	if !reflect.DeepEqual(a.Candidates, b.Candidates) {
		t.Errorf("same seed gave different moves: %v vs %v", a.Candidates, b.Candidates)
	}
}

func TestNeverMoreThanCount(t *testing.T) {
	m := New(1)
	if got := propose(t, m, 3, legalMoves()).Candidates; len(got) != 3 {
		t.Errorf("Count 3 gave %d moves", len(got))
	}
	if got := propose(t, m, 20, []string{"Nf3", "e4", "d4", "c4"}).Candidates; len(got) != 4 {
		t.Errorf("Count 20 with 4 legal gave %d moves, want 4", len(got))
	}
	if got := propose(t, m, 0, legalMoves()).Candidates; len(got) != 0 {
		t.Errorf("Count 0 gave %d moves", len(got))
	}
}

func TestMovesAreDistinctAndFromLegal(t *testing.T) {
	legal := legalMoves()
	in := make(map[string]bool, len(legal))
	for _, m := range legal {
		in[m] = true
	}
	got := propose(t, New(7), len(legal), legal).Candidates
	if len(got) != len(legal) {
		t.Fatalf("got %d moves, want %d", len(got), len(legal))
	}
	seen := map[string]bool{}
	for _, m := range got {
		if !in[m] {
			t.Errorf("move %q is not in Legal", m)
		}
		if seen[m] {
			t.Errorf("move %q returned twice", m)
		}
		seen[m] = true
	}
}

func TestLabels(t *testing.T) {
	reply := propose(t, New(3), 1, legalMoves())
	if reply.Route != "random" || reply.Backend != "random" || reply.Model != "random" {
		t.Errorf("labels = %q %q %q, want all random", reply.Route, reply.Backend, reply.Model)
	}
}
