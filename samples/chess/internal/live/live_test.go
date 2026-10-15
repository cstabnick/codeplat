// Package live_test plays chess against the real opponent: the LLM router for
// the moves and the decide server for the decisions. The test skips unless
// CHESS_LIVE=1 is set, so that `go test ./...` never reaches a model. Use
// `make live` to run it.
package live_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"chess/internal/app"
	"chess/internal/config"
	"chess/internal/core"
)

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("CHESS_LIVE") != "1" {
		t.Skip("set CHESS_LIVE=1 to play against the real router and decide server")
	}
}

// TestLiveOpponentMove starts a game as White against the dev configuration,
// plays e4 and checks that the opponent answered with a move chosen by a model.
func TestLiveOpponentMove(t *testing.T) {
	requireLive(t)
	ctx := context.Background()

	cfg, err := config.Load("dev")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Store = config.Store{Kind: config.StoreMemory}

	svc, closeFn, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeFn(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	g, err := svc.NewGame(ctx, core.White, "normal")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	g, err = svc.Play(ctx, g.ID, "e4")
	if err != nil {
		t.Fatalf("Play e4: %v", err)
	}

	if len(g.Moves) != 2 {
		t.Fatalf("moves = %d, want 2 (the human's e4 and the opponent's reply)", len(g.Moves))
	}
	turn := g.Moves[1].Turn
	if turn == nil {
		t.Fatal("opponent move has no Turn record")
	}
	if turn.Model == "" || turn.Model == "random" {
		t.Errorf("Turn.Model = %q, want a model name from the router", turn.Model)
	}

	out, err := json.MarshalIndent(turn, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("opponent move %s (%s) turn:\n%s", g.Moves[1].UCI, g.Moves[1].SAN, out)
}
