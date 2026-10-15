package board

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"chess/internal/core"
)

func replay(t *testing.T, moves ...string) core.Position {
	t.Helper()
	pos, err := New().Replay(moves)
	if err != nil {
		t.Fatalf("Replay(%v): %v", moves, err)
	}
	return pos
}

func TestReplayStartPosition(t *testing.T) {
	pos := replay(t)
	if len(pos.Legal) != 20 {
		t.Fatalf("start position has %d legal moves, want 20", len(pos.Legal))
	}
	if len(pos.LegalSAN) != len(pos.Legal) {
		t.Fatalf("LegalSAN has %d entries, Legal has %d", len(pos.LegalSAN), len(pos.Legal))
	}
	if !reflect.DeepEqual(pos.Legal, sortedCopy(pos.Legal)) {
		t.Errorf("Legal is not sorted: %v", pos.Legal)
	}
	if pos.Turn != core.White {
		t.Errorf("Turn = %q, want white", pos.Turn)
	}
	if pos.Status != core.StatusActive || pos.Method != "" || pos.InCheck {
		t.Errorf("start status = %q method %q inCheck %v", pos.Status, pos.Method, pos.InCheck)
	}
	if pos.WhiteMaterial != 39 || pos.BlackMaterial != 39 {
		t.Errorf("material = %d/%d, want 39/39", pos.WhiteMaterial, pos.BlackMaterial)
	}
	if pos.Officers != 14 {
		t.Errorf("Officers = %d, want 14", pos.Officers)
	}
	if !strings.HasPrefix(pos.FEN, "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w") {
		t.Errorf("FEN = %q", pos.FEN)
	}
}

func TestReplayIllegalMove(t *testing.T) {
	_, err := New().Replay([]string{"e2e5"})
	if !errors.Is(err, core.ErrIllegal) {
		t.Fatalf("err = %v, want ErrIllegal", err)
	}
}

func TestReplayHistoryAndCheckmate(t *testing.T) {
	pos := replay(t, "f2f3", "e7e5", "g2g4", "d8h4")
	if pos.Status != core.StatusBlackWon {
		t.Errorf("Status = %q, want black_won", pos.Status)
	}
	if pos.Method != "checkmate" {
		t.Errorf("Method = %q, want checkmate", pos.Method)
	}
	if !pos.InCheck {
		t.Errorf("InCheck = false, want true after fool's mate")
	}
	if len(pos.Legal) != 0 {
		t.Errorf("Legal = %v, want none after the game ends", pos.Legal)
	}
	want := []string{"f3", "e5", "g4", "Qh4"}
	if !reflect.DeepEqual(pos.History, want) {
		t.Errorf("History = %v, want %v", pos.History, want)
	}
	if pos.Turn != core.White {
		t.Errorf("Turn = %q, want white", pos.Turn)
	}
}

func TestParse(t *testing.T) {
	rules := New()
	start := []string{}
	cases := []struct {
		name  string
		moves []string
		text  string
		want  string
	}{
		{"san pawn", start, "e4", "e2e4"},
		{"uci pawn", start, "e2e4", "e2e4"},
		{"san knight", start, "Nf3", "g1f3"},
		{"trailing marks", start, " Nf3+! ", "g1f3"},
		{"castle", []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1e2", "g8f6"}, "0-0", "e1g1"},
		{"castle letters", []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1e2", "g8f6"}, "O-O", "e1g1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := rules.Parse(c.moves, c.text)
			if err != nil {
				t.Fatalf("Parse(%q): %v", c.text, err)
			}
			if got != c.want {
				t.Errorf("Parse(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestParseIllegal(t *testing.T) {
	_, err := New().Parse(nil, "e5")
	if !errors.Is(err, core.ErrIllegal) {
		t.Fatalf("err = %v, want ErrIllegal", err)
	}
}

func TestFeaturesCapture(t *testing.T) {
	// 1.e4 d5 2.exd5: white captures a pawn, and the queen on d8 recaptures on d5.
	f, err := New().Features([]string{"e2e4", "d7d5"}, "e4d5")
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	want := core.Features{
		Capture:       true,
		CapturedValue: 1,
		MovedValue:    1,
		Attacked:      true,
	}
	if f != want {
		t.Errorf("Features = %+v, want %+v", f, want)
	}
}

func TestFeaturesQuietMoveNotAttacked(t *testing.T) {
	f, err := New().Features(nil, "e2e4")
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if f.Capture || f.Check || f.Mate || f.Promotion || f.Castle || f.Attacked {
		t.Errorf("Features = %+v, want no flags set", f)
	}
	if f.MovedValue != 1 {
		t.Errorf("MovedValue = %d, want 1", f.MovedValue)
	}
}

func TestFeaturesCastle(t *testing.T) {
	moves := []string{"e2e4", "e7e5", "g1f3", "b8c6", "f1e2", "g8f6"}
	f, err := New().Features(moves, "e1g1")
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if !f.Castle || f.MovedValue != 0 {
		t.Errorf("Features = %+v, want castle with king moved value 0", f)
	}
}

func TestFeaturesCheckmate(t *testing.T) {
	f, err := New().Features([]string{"f2f3", "e7e5", "g2g4"}, "d8h4")
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if !f.Check || !f.Mate {
		t.Errorf("Features = %+v, want check and mate", f)
	}
}

func TestFeaturesIllegal(t *testing.T) {
	_, err := New().Features(nil, "e2e5")
	if !errors.Is(err, core.ErrIllegal) {
		t.Fatalf("err = %v, want ErrIllegal", err)
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
