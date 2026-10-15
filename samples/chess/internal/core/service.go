package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// maxAttempts is how many times the opponent may ask its mover for a legal move.
const maxAttempts = 5

// Service runs games: it checks the human's input, asks the ports for the
// opponent's move and saves the result.
type Service struct {
	store   Store
	rules   Rules
	mover   Mover
	decider Decider
	now     func() time.Time
	intn    func(n int) int

	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

// NewService wires the ports together. Nil Options fields default to the wall clock and math/rand/v2.
func NewService(store Store, rules Rules, mover Mover, decider Decider, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	intn := opts.Intn
	if intn == nil {
		intn = rand.IntN
	}
	return &Service{
		store:   store,
		rules:   rules,
		mover:   mover,
		decider: decider,
		now:     now,
		intn:    intn,
		locks:   map[int64]*sync.Mutex{},
	}
}

// NewGame starts a game for the human colour. If the human plays Black the opponent moves first.
func (s *Service) NewGame(ctx context.Context, human Color, difficulty string) (Game, error) {
	if human != White && human != Black {
		return Game{}, fmt.Errorf("%w: human must be white or black", ErrInvalid)
	}
	if difficulty == "" {
		difficulty = "normal"
	}
	if !validDifficulty(difficulty) {
		return Game{}, fmt.Errorf("%w: difficulty must be easy, normal or hard", ErrInvalid)
	}
	now := s.now()
	g, err := s.store.Create(ctx, Game{
		Human:      human,
		Difficulty: difficulty,
		Moves:      []Move{},
		Status:     StatusActive,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		return Game{}, err
	}
	if human == Black {
		pos, err := s.rules.Replay(uciList(g.Moves))
		if err != nil {
			return Game{}, err
		}
		g, err = s.opponentMove(ctx, g, pos)
		if err != nil {
			return Game{}, err
		}
		g.UpdatedAt = now
		if err := s.store.Save(ctx, g); err != nil {
			return Game{}, err
		}
	}
	return s.view(g)
}

// Game returns one game with its derived fields.
func (s *Service) Game(ctx context.Context, id int64) (Game, error) {
	g, err := s.store.Get(ctx, id)
	if err != nil {
		return Game{}, err
	}
	return s.view(g)
}

// Games returns every game with its derived fields. The result is never nil.
func (s *Service) Games(ctx context.Context) ([]Game, error) {
	list, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Game, 0, len(list))
	for _, g := range list {
		v, err := s.view(g)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Play makes the human's move, then the opponent's reply if the game is still active.
func (s *Service) Play(ctx context.Context, id int64, move string) (Game, error) {
	defer s.lockGame(id)()
	g, err := s.store.Get(ctx, id)
	if err != nil {
		return Game{}, err
	}
	if g.Status != StatusActive {
		return Game{}, fmt.Errorf("%w: game %d is over", ErrConflict, id)
	}
	pos, err := s.rules.Replay(uciList(g.Moves))
	if err != nil {
		return Game{}, err
	}
	if pos.Turn != g.Human {
		return Game{}, fmt.Errorf("%w: it is not the human's move", ErrConflict)
	}
	uci, err := s.rules.Parse(uciList(g.Moves), move)
	if err != nil {
		return Game{}, err
	}
	after, err := s.rules.Replay(append(uciList(g.Moves), uci))
	if err != nil {
		return Game{}, err
	}
	g.Moves = append(g.Moves, Move{UCI: uci, SAN: after.History[len(after.History)-1], By: g.Human})
	g.Status, g.Method = after.Status, after.Method
	if g.Status == StatusActive {
		g, err = s.opponentMove(ctx, g, after)
		if err != nil {
			return Game{}, err
		}
	}
	g.UpdatedAt = s.now()
	if err := s.store.Save(ctx, g); err != nil {
		return Game{}, err
	}
	return s.view(g)
}

// Resign ends an active game with a win for the opponent.
func (s *Service) Resign(ctx context.Context, id int64) (Game, error) {
	defer s.lockGame(id)()
	g, err := s.store.Get(ctx, id)
	if err != nil {
		return Game{}, err
	}
	if g.Status != StatusActive {
		return Game{}, fmt.Errorf("%w: game %d is over", ErrConflict, id)
	}
	g.Status, g.Method = winnerStatus(g.Human.Other()), "resignation"
	g.UpdatedAt = s.now()
	if err := s.store.Save(ctx, g); err != nil {
		return Game{}, err
	}
	return s.view(g)
}

// opponentMove plays the opponent's move in pos (the position with the opponent to move)
// and returns the game with that move appended and its status updated. A resignation
// by the opponent ends the game without a move, so its Turn record is not kept.
func (s *Service) opponentMove(ctx context.Context, g Game, pos Position) (Game, error) {
	opp := g.Human.Other()
	turn := &Turn{}

	policy := s.decide(ctx, DecisionPolicy, policyInput(g, pos), turn)
	route, _ := policy["route"].(string)
	persona, _ := policy["persona"].(string)
	temperature, _ := numberOf(policy["temperature"])
	count := 3
	if n, ok := numberOf(policy["candidates"]); ok {
		count = min(max(int(n), 1), 5)
	}

	var legal []string
	resign := false
	feedback := ""
	for attempt := 1; ; attempt++ {
		turn.Attempts = attempt
		reply, err := s.mover.Propose(ctx, MoveRequest{
			FEN:         pos.FEN,
			Color:       opp,
			History:     pos.History,
			Legal:       pos.LegalSAN,
			Count:       count,
			Route:       route,
			Temperature: temperature,
			Persona:     persona,
			Feedback:    feedback,
		})
		reason := "illegal"
		if err != nil {
			turn.Errors = append(turn.Errors, fmt.Sprintf("propose: %v", err))
			reason = "no_move"
		} else {
			turn.Candidates = reply.Candidates
			turn.Route, turn.Backend, turn.Model = reply.Route, reply.Backend, reply.Model
			legal = s.legalCandidates(uciList(g.Moves), reply.Candidates, turn)
			if len(legal) > 0 {
				break
			}
		}
		if attempt >= maxAttempts {
			turn.Fallback = "random"
			break
		}
		out := s.decide(ctx, DecisionIllegal, map[string]any{
			"attempt":     attempt,
			"reason":      reason,
			"difficulty":  g.Difficulty,
			"legal_count": len(pos.Legal),
		}, turn)
		action, _ := out["action"].(string)
		if action == "retry" {
			feedback = fmt.Sprintf(`Your answer "%s" had no legal move. Pick one of the legal moves listed.`, excerpt(reply.Raw, 200))
			continue
		}
		if action == "resign" {
			turn.Fallback = "resign"
			resign = true
		} else {
			turn.Fallback = "random"
		}
		break
	}

	if resign {
		g.Status, g.Method = winnerStatus(g.Human), "resignation"
		return g, nil
	}

	var uci string
	if len(legal) > 0 {
		chosen, err := s.choose(ctx, g, legal, turn)
		if err != nil {
			return Game{}, err
		}
		uci = chosen
	} else {
		if len(pos.Legal) == 0 {
			return Game{}, fmt.Errorf("%w: no legal move for %s", ErrConflict, opp)
		}
		uci = pos.Legal[s.intn(len(pos.Legal))]
	}

	g.Moves = append(g.Moves, Move{UCI: uci, By: opp, Turn: turn})
	after, err := s.rules.Replay(uciList(g.Moves))
	if err != nil {
		return Game{}, err
	}
	g.Moves[len(g.Moves)-1].SAN = after.History[len(after.History)-1]
	g.Status, g.Method = after.Status, after.Method
	return g, nil
}

// choose scores each legal candidate with the choice decision and returns the best.
// Ties go to the lower rank. Each candidate with a numeric score is recorded in
// turn.Scores by UCI; a candidate without one is left out of it. A single
// candidate is not scored, so turn.Scores stays nil.
func (s *Service) choose(ctx context.Context, g Game, legal []string, turn *Turn) (string, error) {
	if len(legal) == 1 {
		return legal[0], nil
	}
	best, bestScore := 0, math.Inf(-1)
	for rank, uci := range legal {
		f, err := s.rules.Features(uciList(g.Moves), uci)
		if err != nil {
			return "", err
		}
		in := featuresInput(f)
		in["rank"] = rank
		in["difficulty"] = g.Difficulty
		out := s.decide(ctx, DecisionChoice, in, turn)
		score, ok := numberOf(out["score"])
		if !ok {
			// Negative infinity keeps this candidate from winning, but it is not
			// stored: encoding/json cannot encode it, so saving the game would fail.
			score = math.Inf(-1)
		} else {
			if turn.Scores == nil {
				turn.Scores = map[string]float64{}
			}
			turn.Scores[uci] = score
		}
		if score > bestScore {
			best, bestScore = rank, score
		}
	}
	return legal[best], nil
}

// legalCandidates parses the raw texts and keeps the legal moves, unique and in order.
func (s *Service) legalCandidates(moves []string, texts []string, turn *Turn) []string {
	seen := map[string]bool{}
	var out []string
	for _, text := range texts {
		uci, err := s.rules.Parse(moves, text)
		if err != nil {
			if !errors.Is(err, ErrIllegal) {
				turn.Errors = append(turn.Errors, fmt.Sprintf("parse %q: %v", text, err))
			}
			continue
		}
		if seen[uci] {
			continue
		}
		seen[uci] = true
		out = append(out, uci)
	}
	return out
}

// decide asks the decider; on error it uses FixedDecider's answer and records the error.
func (s *Service) decide(ctx context.Context, key string, input map[string]any, turn *Turn) map[string]any {
	out, err := s.decider.Decide(ctx, key, input)
	if err != nil {
		turn.Errors = append(turn.Errors, fmt.Sprintf("decide %s: %v", key, err))
		out, _ = FixedDecider{}.Decide(ctx, key, input)
	}
	return out
}

// view fills the derived fields from a replay of the stored moves.
func (s *Service) view(g Game) (Game, error) {
	pos, err := s.rules.Replay(uciList(g.Moves))
	if err != nil {
		return Game{}, err
	}
	g.FEN = pos.FEN
	g.Turn = pos.Turn
	g.Legal = []string{}
	if g.Status == StatusActive {
		g.Legal = append(g.Legal, pos.Legal...)
	}
	return g, nil
}

// lockGame locks one game and returns its unlock function.
func (s *Service) lockGame(id int64) func() {
	s.mu.Lock()
	l := s.locks[id]
	if l == nil {
		l = &sync.Mutex{}
		s.locks[id] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func policyInput(g Game, pos Position) map[string]any {
	moveNumber := len(g.Moves)/2 + 1
	phase := "middlegame"
	switch {
	case moveNumber <= 10:
		phase = "opening"
	case pos.Officers <= 6:
		phase = "endgame"
	}
	opponentMaterial, humanMaterial := pos.WhiteMaterial, pos.BlackMaterial
	if g.Human.Other() == Black {
		opponentMaterial, humanMaterial = pos.BlackMaterial, pos.WhiteMaterial
	}
	return map[string]any{
		"difficulty":       g.Difficulty,
		"phase":            phase,
		"move_number":      moveNumber,
		"material_balance": opponentMaterial - humanMaterial,
		"in_check":         pos.InCheck,
		"legal_count":      len(pos.Legal),
	}
}

func featuresInput(f Features) map[string]any {
	return map[string]any{
		"capture":        f.Capture,
		"captured_value": f.CapturedValue,
		"check":          f.Check,
		"mate":           f.Mate,
		"promotion":      f.Promotion,
		"castle":         f.Castle,
		"moved_value":    f.MovedValue,
		"attacked":       f.Attacked,
	}
}

func uciList(moves []Move) []string {
	out := make([]string, 0, len(moves))
	for _, m := range moves {
		out = append(out, m.UCI)
	}
	return out
}

func winnerStatus(winner Color) Status {
	if winner == White {
		return StatusWhiteWon
	}
	return StatusBlackWon
}

func validDifficulty(d string) bool {
	return d == "easy" || d == "normal" || d == "hard"
}

func excerpt(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// numberOf reads a decision value that may arrive as an int or a float.
func numberOf(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	}
	return 0, false
}
