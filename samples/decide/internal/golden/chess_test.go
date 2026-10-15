// Package golden checks the chess decision models against fixed inputs.
package golden

import (
	"context"
	"reflect"
	"testing"

	"decide/internal/apptest"
)

// checkFields fails the test for every key in want whose value in got differs.
func checkFields(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("result has no %q, want %#v", k, w)
			continue
		}
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %#v, want %#v", k, g, w)
		}
	}
}

func TestChessOpponentPolicy(t *testing.T) {
	svc := apptest.New(t)
	ctx := context.Background()
	tests := []struct {
		name  string
		input map[string]any
		want  map[string]any
	}{
		{
			name:  "normal opening",
			input: map[string]any{"difficulty": "normal", "phase": "opening", "in_check": false},
			want: map[string]any{
				"route":       "quick",
				"temperature": 0.5,
				"candidates":  3.0,
				"persona":     "You play sound openings: develop pieces, control the centre, castle early.",
			},
		},
		{
			name:  "easy middlegame",
			input: map[string]any{"difficulty": "easy", "phase": "middlegame", "in_check": false},
			want:  map[string]any{"route": "quick", "temperature": 1.0, "candidates": 1.0},
		},
		{
			name:  "normal middlegame in check",
			input: map[string]any{"difficulty": "normal", "phase": "middlegame", "in_check": true},
			want:  map[string]any{"route": "general", "temperature": 0.2, "candidates": 3.0},
		},
		{
			name:  "hard endgame",
			input: map[string]any{"difficulty": "hard", "phase": "endgame", "in_check": false},
			want:  map[string]any{"route": "general", "temperature": 0.2, "candidates": 5.0},
		},
		{
			name:  "normal endgame",
			input: map[string]any{"difficulty": "normal", "phase": "endgame", "in_check": false},
			want:  map[string]any{"route": "general", "temperature": 0.5, "candidates": 3.0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.Evaluate(ctx, "chess-opponent-policy", tc.input)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			checkFields(t, got, tc.want)
		})
	}
}

func TestChessIllegalMove(t *testing.T) {
	svc := apptest.New(t)
	ctx := context.Background()
	tests := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"first illegal retries", map[string]any{"attempt": 1, "reason": "illegal", "difficulty": "normal"}, "retry"},
		{"second no move gives up", map[string]any{"attempt": 2, "reason": "no_move", "difficulty": "normal"}, "random"},
		{"first no move retries", map[string]any{"attempt": 1, "reason": "no_move", "difficulty": "normal"}, "retry"},
		{"third illegal easy resigns", map[string]any{"attempt": 3, "reason": "illegal", "difficulty": "easy"}, "resign"},
		{"third illegal normal is random", map[string]any{"attempt": 3, "reason": "illegal", "difficulty": "normal"}, "random"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.Evaluate(ctx, "chess-illegal-move", tc.input)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			checkFields(t, got, map[string]any{"action": tc.want})
		})
	}
}

func TestChessMoveChoice(t *testing.T) {
	svc := apptest.New(t)
	ctx := context.Background()
	tests := []struct {
		name  string
		input map[string]any
		want  float64
	}{
		{
			name: "capture of a value 3 piece",
			input: map[string]any{
				"capture": true, "captured_value": 3, "check": false, "mate": false,
				"promotion": false, "castle": false, "moved_value": 1, "attacked": false,
				"rank": 0, "difficulty": "normal",
			},
			want: 30,
		},
		{
			name: "check onto an attacked square",
			input: map[string]any{
				"capture": false, "captured_value": 0, "check": true, "mate": false,
				"promotion": false, "castle": false, "moved_value": 9, "attacked": true,
				"rank": 1, "difficulty": "normal",
			},
			want: -88,
		},
		{
			name: "mate",
			input: map[string]any{
				"capture": false, "captured_value": 0, "check": false, "mate": true,
				"promotion": false, "castle": false, "moved_value": 0, "attacked": false,
				"rank": 2, "difficulty": "normal",
			},
			want: 998,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.Evaluate(ctx, "chess-move-choice", tc.input)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			checkFields(t, got, map[string]any{"score": tc.want})
		})
	}
}
