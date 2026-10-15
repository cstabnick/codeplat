// Package core holds the chess game rules that do not depend on any transport
// or implementation: the types, the ports they use, and the Service that runs a game.
package core

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
	ErrIllegal  = errors.New("illegal move")
	ErrConflict = errors.New("conflict")
)

type Color string

const (
	White Color = "white"
	Black Color = "black"
)

// Other returns the opposite colour.
func (c Color) Other() Color {
	if c == White {
		return Black
	}
	return White
}

type Status string

const (
	StatusActive   Status = "active"
	StatusWhiteWon Status = "white_won"
	StatusBlackWon Status = "black_won"
	StatusDraw     Status = "draw"
)

type Game struct {
	ID         int64     `json:"id"`
	Human      Color     `json:"human"`
	Difficulty string    `json:"difficulty"` // easy, normal, hard
	Moves      []Move    `json:"moves"`
	Status     Status    `json:"status"`
	Method     string    `json:"method,omitempty"` // checkmate, stalemate, resignation, ...
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Derived; the Service fills them on every Game it returns.
	FEN   string   `json:"fen"`
	Turn  Color    `json:"turn"`
	Legal []string `json:"legal"` // UCI moves for the side to move; empty when the game is over
}

type Move struct {
	UCI  string `json:"uci"`
	SAN  string `json:"san"`
	By   Color  `json:"by"`
	Turn *Turn  `json:"turn,omitempty"` // set on opponent moves
}

// Turn records how the opponent chose a move.
type Turn struct {
	Route      string             `json:"route,omitempty"`
	Backend    string             `json:"backend,omitempty"`
	Model      string             `json:"model,omitempty"`
	Policy     map[string]any     `json:"policy,omitempty"`
	Candidates []string           `json:"candidates,omitempty"` // raw texts of the last proposal
	Scores     map[string]float64 `json:"scores,omitempty"`     // by UCI
	Attempts   int                `json:"attempts"`
	Fallback   string             `json:"fallback,omitempty"` // "", "random" or "resign"
	Errors     []string           `json:"errors,omitempty"`
}

type Position struct {
	FEN           string
	Turn          Color
	Legal         []string // UCI, sorted
	LegalSAN      []string // SAN, same order as Legal
	History       []string // SAN of every move played
	InCheck       bool     // the side to move is in check
	Status        Status
	Method        string
	WhiteMaterial int // pawn 1, knight 3, bishop 3, rook 5, queen 9
	BlackMaterial int
	Officers      int // knights, bishops, rooks and queens of both colours
}

type Features struct {
	Capture       bool `json:"capture"`
	CapturedValue int  `json:"captured_value"`
	Check         bool `json:"check"`
	Mate          bool `json:"mate"`
	Promotion     bool `json:"promotion"`
	Castle        bool `json:"castle"`
	MovedValue    int  `json:"moved_value"` // king 0
	Attacked      bool `json:"attacked"`    // after the move, the other side has a legal move to its square
}

type Store interface {
	Create(ctx context.Context, g Game) (Game, error) // assigns ID 1, 2, ...
	Get(ctx context.Context, id int64) (Game, error)  // ErrNotFound
	List(ctx context.Context) ([]Game, error)         // UpdatedAt newest first, then ID descending
	Save(ctx context.Context, g Game) error           // replaces by ID; ErrNotFound
}

type Rules interface {
	// Replay plays UCI moves from the start position. An illegal move wraps ErrIllegal.
	Replay(moves []string) (Position, error)
	// Parse reads text (UCI like "e2e4" or SAN like "Nf3") as a legal move after moves; returns UCI or wraps ErrIllegal.
	Parse(moves []string, text string) (string, error)
	Features(moves []string, uci string) (Features, error)
}

type MoveRequest struct {
	FEN         string
	Color       Color
	History     []string // SAN
	Legal       []string // SAN
	Count       int
	Route       string // "" means the opponent's configured model
	Temperature float64
	Persona     string
	Feedback    string // why the previous answer failed; "" on the first attempt
}

type MoveReply struct {
	Candidates []string // raw move texts, best first
	Route      string
	Backend    string
	Model      string
	Raw        string
}

type Mover interface {
	Propose(ctx context.Context, req MoveRequest) (MoveReply, error)
}

type Decider interface {
	Decide(ctx context.Context, key string, input map[string]any) (map[string]any, error)
}

const (
	DecisionPolicy  = "chess-opponent-policy"
	DecisionIllegal = "chess-illegal-move"
	DecisionChoice  = "chess-move-choice"
)

type Options struct {
	Now  func() time.Time
	Intn func(n int) int // must be safe for concurrent use
}
