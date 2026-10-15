// Package board implements core.Rules on top of github.com/notnil/chess.
package board

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/notnil/chess"

	"chess/internal/core"
)

// Rules is the chess rules engine. It holds no state.
type Rules struct{}

var _ core.Rules = Rules{}

// New returns the rules engine.
func New() Rules {
	return Rules{}
}

// Replay plays UCI moves from the start position and describes the result.
func (Rules) Replay(moves []string) (core.Position, error) {
	g, history, err := play(moves)
	if err != nil {
		return core.Position{}, err
	}
	pos := g.Position()

	type pair struct{ uci, san string }
	var pairs []pair
	for _, mv := range pos.ValidMoves() {
		pairs = append(pairs, pair{
			uci: mv.String(),
			san: chess.AlgebraicNotation{}.Encode(pos, mv),
		})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].uci < pairs[j].uci })
	legal := make([]string, 0, len(pairs))
	legalSAN := make([]string, 0, len(pairs))
	for _, p := range pairs {
		legal = append(legal, p.uci)
		legalSAN = append(legalSAN, p.san)
	}

	white, black := material(pos.Board())
	return core.Position{
		FEN:           g.FEN(),
		Turn:          colorOf(pos.Turn()),
		Legal:         legal,
		LegalSAN:      legalSAN,
		History:       history,
		InCheck:       inCheck(g),
		Status:        statusOf(g.Outcome()),
		Method:        methodName(g.Method()),
		WhiteMaterial: white,
		BlackMaterial: black,
		Officers:      officers(pos.Board()),
	}, nil
}

// Parse reads text as a legal move after moves and returns it in UCI.
func (Rules) Parse(moves []string, text string) (string, error) {
	g, _, err := play(moves)
	if err != nil {
		return "", err
	}
	pos := g.Position()

	s := strings.TrimSpace(text)
	s = strings.TrimRight(s, "+#!?")
	switch s {
	case "0-0":
		s = "O-O"
	case "0-0-0":
		s = "O-O-O"
	}

	var candidates []*chess.Move
	if mv, err := (chess.UCINotation{}).Decode(pos, s); err == nil {
		candidates = append(candidates, mv)
	}
	if mv, err := (chess.AlgebraicNotation{}).Decode(pos, s); err == nil {
		candidates = append(candidates, mv)
	}
	for _, mv := range candidates {
		if valid := findLegal(pos, mv.String()); valid != nil {
			return chess.UCINotation{}.Encode(pos, valid), nil
		}
	}
	return "", fmt.Errorf("%w: %s", core.ErrIllegal, text)
}

// Features describes the move uci played after moves.
func (Rules) Features(moves []string, uci string) (core.Features, error) {
	g, _, err := play(moves)
	if err != nil {
		return core.Features{}, err
	}
	pos := g.Position()
	mv := findLegal(pos, uci)
	if mv == nil {
		return core.Features{}, fmt.Errorf("%w: %s", core.ErrIllegal, uci)
	}

	board := pos.Board()
	f := core.Features{
		MovedValue: pieceValue(board.Piece(mv.S1())),
		Check:      mv.HasTag(chess.Check),
		Promotion:  mv.Promo() != chess.NoPieceType,
		Castle:     mv.HasTag(chess.KingSideCastle) || mv.HasTag(chess.QueenSideCastle),
	}
	switch {
	case mv.HasTag(chess.EnPassant):
		f.Capture = true
		f.CapturedValue = 1
	case mv.HasTag(chess.Capture):
		f.Capture = true
		f.CapturedValue = pieceValue(board.Piece(mv.S2()))
	}

	if err := g.Move(mv); err != nil {
		return core.Features{}, fmt.Errorf("%w: %s", core.ErrIllegal, uci)
	}
	f.Mate = g.Method() == chess.Checkmate
	for _, next := range g.Position().ValidMoves() {
		if next.S2() == mv.S2() {
			f.Attacked = true
			break
		}
	}
	return f, nil
}

// play replays UCI moves from the start and returns the game and the SAN of
// each move as it was played.
func play(moves []string) (*chess.Game, []string, error) {
	g := chess.NewGame()
	history := make([]string, 0, len(moves))
	for _, m := range moves {
		mv, err := (chess.UCINotation{}).Decode(g.Position(), m)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s", core.ErrIllegal, m)
		}
		san := chess.AlgebraicNotation{}.Encode(g.Position(), mv)
		if err := g.Move(mv); err != nil {
			return nil, nil, fmt.Errorf("%w: %s", core.ErrIllegal, m)
		}
		history = append(history, san)
	}
	return g, history, nil
}

// findLegal returns the legal move in pos whose UCI text is uci, or nil.
func findLegal(pos *chess.Position, uci string) *chess.Move {
	for _, mv := range pos.ValidMoves() {
		if mv.String() == uci {
			return mv
		}
	}
	return nil
}

func inCheck(g *chess.Game) bool {
	moves := g.Moves()
	return len(moves) > 0 && moves[len(moves)-1].HasTag(chess.Check)
}

func statusOf(o chess.Outcome) core.Status {
	switch o {
	case chess.WhiteWon:
		return core.StatusWhiteWon
	case chess.BlackWon:
		return core.StatusBlackWon
	case chess.Draw:
		return core.StatusDraw
	default:
		return core.StatusActive
	}
}

// methodName returns the snake_case name of m, or "" for no method.
func methodName(m chess.Method) string {
	if m == chess.NoMethod {
		return ""
	}
	var b strings.Builder
	for i, r := range m.String() {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func colorOf(c chess.Color) core.Color {
	if c == chess.White {
		return core.White
	}
	return core.Black
}

// pieceValue returns the value of p as in core.Position: pawn 1, knight 3,
// bishop 3, rook 5, queen 9, king 0. An empty square is 0.
func pieceValue(p chess.Piece) int {
	if p == chess.NoPiece {
		return 0
	}
	switch p.Type() {
	case chess.Pawn:
		return 1
	case chess.Knight, chess.Bishop:
		return 3
	case chess.Rook:
		return 5
	case chess.Queen:
		return 9
	default:
		return 0
	}
}

// material returns the material value of each colour on the board.
func material(b *chess.Board) (white, black int) {
	for _, p := range b.SquareMap() {
		if p.Color() == chess.White {
			white += pieceValue(p)
		} else if p.Color() == chess.Black {
			black += pieceValue(p)
		}
	}
	return white, black
}

// officers counts the knights, bishops, rooks and queens of both colours.
func officers(b *chess.Board) int {
	n := 0
	for _, p := range b.SquareMap() {
		switch p.Type() {
		case chess.Knight, chess.Bishop, chess.Rook, chess.Queen:
			n++
		}
	}
	return n
}
