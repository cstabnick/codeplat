package app_test

import (
	"context"
	"path/filepath"
	"testing"

	"chess/internal/app"
	"chess/internal/apptest"
	"chess/internal/config"
	"chess/internal/core"
)

func TestNewGameWhitePlayE4(t *testing.T) {
	svc := apptest.New(t)
	ctx := context.Background()

	g, err := svc.NewGame(ctx, core.White, "")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	g, err = svc.Play(ctx, g.ID, "e4")
	if err != nil {
		t.Fatalf("Play e4: %v", err)
	}
	if len(g.Moves) != 2 {
		t.Fatalf("moves = %d, want 2", len(g.Moves))
	}
	if g.Moves[0].By != core.White || g.Moves[0].UCI != "e2e4" {
		t.Errorf("first move = %+v, want white e2e4", g.Moves[0])
	}
	reply := g.Moves[1]
	if reply.By != core.Black {
		t.Errorf("second move by %q, want black", reply.By)
	}
	if reply.Turn == nil {
		t.Fatal("opponent move has no Turn record")
	}
	if reply.Turn.Model != "random" {
		t.Errorf("Turn.Model = %q, want random", reply.Turn.Model)
	}
}

func TestNewGameBlackOpponentMovesFirst(t *testing.T) {
	svc := apptest.New(t)

	g, err := svc.NewGame(context.Background(), core.Black, "")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	if len(g.Moves) != 1 {
		t.Fatalf("moves = %d, want 1", len(g.Moves))
	}
	if g.Moves[0].By != core.White {
		t.Errorf("first move by %q, want white", g.Moves[0].By)
	}
}

func TestSQLiteConfigBuildsAndCloses(t *testing.T) {
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Store = config.Store{Kind: config.StoreSQLite, Path: filepath.Join(t.TempDir(), "chess.db")}

	svc, closeFn, err := app.New(cfg)
	if err != nil {
		t.Fatalf("New with sqlite: %v", err)
	}
	if _, err := svc.NewGame(context.Background(), core.White, ""); err != nil {
		t.Fatalf("NewGame on sqlite: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUnknownKindFails(t *testing.T) {
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Store.Kind = "bogus"

	if _, _, err := app.New(cfg); err == nil {
		t.Fatal("New with unknown store kind: want error, got nil")
	}
}
