package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

// testLegal is the fixed legal list every fake position reports, sorted as UCI.
var testLegal = []string{"d2d4", "d7d5", "e2e4", "g1f3"}

// fakeRules reports the same legal moves in every position. Text is accepted as
// UCI ("e2e4") or as the fake SAN ("san-e2e4"); anything else is illegal.
type fakeRules struct {
	legal    []string
	features map[string]Features
}

func (r fakeRules) Replay(moves []string) (Position, error) {
	turn := White
	if len(moves)%2 == 1 {
		turn = Black
	}
	pos := Position{
		FEN:           fmt.Sprintf("fen-%d", len(moves)),
		Turn:          turn,
		Legal:         append([]string(nil), r.legal...),
		Status:        StatusActive,
		History:       []string{},
		WhiteMaterial: 39,
		BlackMaterial: 39,
		Officers:      8,
	}
	for _, m := range moves {
		pos.History = append(pos.History, "san-"+m)
	}
	for _, u := range r.legal {
		pos.LegalSAN = append(pos.LegalSAN, "san-"+u)
	}
	return pos, nil
}

func (r fakeRules) Parse(moves []string, text string) (string, error) {
	for _, u := range r.legal {
		if text == u || text == "san-"+u {
			return u, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrIllegal, text)
}

func (r fakeRules) Features(moves []string, uci string) (Features, error) {
	return r.features[uci], nil
}

// fakeStore is a map-backed Store.
type fakeStore struct {
	games map[int64]Game
	next  int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{games: map[int64]Game{}}
}

func (s *fakeStore) Create(ctx context.Context, g Game) (Game, error) {
	s.next++
	g.ID = s.next
	s.games[g.ID] = g
	return g, nil
}

func (s *fakeStore) Get(ctx context.Context, id int64) (Game, error) {
	g, ok := s.games[id]
	if !ok {
		return Game{}, ErrNotFound
	}
	return g, nil
}

func (s *fakeStore) List(ctx context.Context) ([]Game, error) {
	var out []Game
	for _, g := range s.games {
		out = append(out, g)
	}
	return out, nil
}

func (s *fakeStore) Save(ctx context.Context, g Game) error {
	if _, ok := s.games[g.ID]; !ok {
		return ErrNotFound
	}
	s.games[g.ID] = g
	return nil
}

// moveStep is one scripted answer from the fakeMover.
type moveStep struct {
	reply MoveReply
	err   error
}

// fakeMover answers Propose from a script and records every request.
type fakeMover struct {
	steps []moveStep
	reqs  []MoveRequest
}

func (m *fakeMover) Propose(ctx context.Context, req MoveRequest) (MoveReply, error) {
	m.reqs = append(m.reqs, req)
	i := len(m.reqs) - 1
	if i >= len(m.steps) {
		return MoveReply{}, errors.New("no scripted reply")
	}
	return m.steps[i].reply, m.steps[i].err
}

// fakeDecider returns canned answers per decision, or an error when one is set.
type fakeDecider struct {
	policy  map[string]any
	illegal map[string]any
	errs    map[string]error
	calls   []string
}

func newFakeDecider() *fakeDecider {
	return &fakeDecider{
		policy:  map[string]any{"route": "", "temperature": 0.5, "persona": "p", "candidates": 3},
		illegal: map[string]any{"action": "random"},
		errs:    map[string]error{},
	}
}

func (d *fakeDecider) Decide(ctx context.Context, key string, in map[string]any) (map[string]any, error) {
	d.calls = append(d.calls, key)
	if err := d.errs[key]; err != nil {
		return nil, err
	}
	switch key {
	case DecisionPolicy:
		return d.policy, nil
	case DecisionIllegal:
		return d.illegal, nil
	case DecisionChoice:
		return map[string]any{"score": in["captured_value"]}, nil
	}
	return nil, fmt.Errorf("unknown decision %q", key)
}

func (d *fakeDecider) count(key string) int {
	n := 0
	for _, c := range d.calls {
		if c == key {
			n++
		}
	}
	return n
}

type fixture struct {
	svc     *Service
	store   *fakeStore
	mover   *fakeMover
	decider *fakeDecider
}

func newFixture(steps ...moveStep) *fixture {
	f := &fixture{store: newFakeStore(), mover: &fakeMover{steps: steps}, decider: newFakeDecider()}
	rules := fakeRules{
		legal:    testLegal,
		features: map[string]Features{"g1f3": {CapturedValue: 5}, "d7d5": {CapturedValue: 2}},
	}
	f.svc = NewService(f.store, rules, f.mover, f.decider, Options{
		Now:  func() time.Time { return fixedNow },
		Intn: func(int) int { return 0 },
	})
	return f
}

func reply(candidates ...string) moveStep {
	return moveStep{reply: MoveReply{Candidates: candidates}}
}

func mustNewGame(t *testing.T, f *fixture, human Color) Game {
	t.Helper()
	g, err := f.svc.NewGame(context.Background(), human, "")
	if err != nil {
		t.Fatalf("NewGame: %v", err)
	}
	return g
}

func mustPlay(t *testing.T, f *fixture, id int64, move string) Game {
	t.Helper()
	g, err := f.svc.Play(context.Background(), id, move)
	if err != nil {
		t.Fatalf("Play(%q): %v", move, err)
	}
	return g
}

func TestNewGameValidation(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	if _, err := f.svc.NewGame(ctx, "green", "normal"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad colour: err = %v, want ErrInvalid", err)
	}
	if _, err := f.svc.NewGame(ctx, White, "impossible"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad difficulty: err = %v, want ErrInvalid", err)
	}
	g, err := f.svc.NewGame(ctx, White, "")
	if err != nil {
		t.Fatalf("NewGame with empty difficulty: %v", err)
	}
	if g.Difficulty != "normal" {
		t.Errorf("Difficulty = %q, want normal", g.Difficulty)
	}
	if g.Status != StatusActive || len(g.Moves) != 0 || g.Turn != White {
		t.Errorf("new white game = status %s, %d moves, turn %s", g.Status, len(g.Moves), g.Turn)
	}
	if len(g.Legal) != len(testLegal) {
		t.Errorf("Legal = %v, want the fixed list", g.Legal)
	}
	if len(f.mover.reqs) != 0 {
		t.Errorf("white human: mover asked %d times, want 0", len(f.mover.reqs))
	}
}

func TestBlackHumanOpponentMovesFirst(t *testing.T) {
	f := newFixture(reply("e2e4"))
	g := mustNewGame(t, f, Black)
	if len(g.Moves) != 1 {
		t.Fatalf("moves = %d, want 1", len(g.Moves))
	}
	m := g.Moves[0]
	if m.UCI != "e2e4" || m.By != White || m.Turn == nil || m.Turn.Attempts != 1 {
		t.Errorf("opponent move = %+v", m)
	}
	if g.Turn != Black {
		t.Errorf("turn = %s, want black (human)", g.Turn)
	}
	stored, err := f.store.Get(context.Background(), g.ID)
	if err != nil || len(stored.Moves) != 1 {
		t.Errorf("stored game not saved after the opponent move: %v, %d moves", err, len(stored.Moves))
	}
}

func TestPlayWrongTurnAndFinishedGame(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	// Stored with the opponent to move for a White human.
	g, err := f.store.Create(ctx, Game{Human: White, Difficulty: "normal", Status: StatusActive,
		Moves: []Move{{UCI: "e2e4", SAN: "san-e2e4", By: White}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Play(ctx, g.ID, "d7d5"); !errors.Is(err, ErrConflict) {
		t.Errorf("wrong turn: err = %v, want ErrConflict", err)
	}
	done, err := f.store.Create(ctx, Game{Human: White, Difficulty: "normal", Status: StatusDraw})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Play(ctx, done.ID, "e2e4"); !errors.Is(err, ErrConflict) {
		t.Errorf("finished game: err = %v, want ErrConflict", err)
	}
}

func TestPlayIllegalHumanMove(t *testing.T) {
	f := newFixture()
	g := mustNewGame(t, f, White)
	_, err := f.svc.Play(context.Background(), g.ID, "zz")
	if !errors.Is(err, ErrIllegal) {
		t.Fatalf("err = %v, want ErrIllegal", err)
	}
	stored, _ := f.store.Get(context.Background(), g.ID)
	if len(stored.Moves) != 0 {
		t.Errorf("illegal move was saved: %d moves", len(stored.Moves))
	}
}

func TestPlayHumanThenOpponent(t *testing.T) {
	f := newFixture(moveStep{reply: MoveReply{Candidates: []string{"d7d5"}, Route: "r", Backend: "b", Model: "m"}})
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	if len(g.Moves) != 2 {
		t.Fatalf("moves = %d, want 2", len(g.Moves))
	}
	human, opp := g.Moves[0], g.Moves[1]
	if human.UCI != "e2e4" || human.SAN != "san-e2e4" || human.By != White || human.Turn != nil {
		t.Errorf("human move = %+v", human)
	}
	if opp.UCI != "d7d5" || opp.By != Black || opp.Turn == nil {
		t.Fatalf("opponent move = %+v", opp)
	}
	if opp.Turn.Route != "r" || opp.Turn.Backend != "b" || opp.Turn.Model != "m" {
		t.Errorf("turn route info = %+v", opp.Turn)
	}
	if g.Turn != White || g.Status != StatusActive {
		t.Errorf("after the exchange turn = %s status = %s", g.Turn, g.Status)
	}
}

func TestOpponentRetryThenSuccess(t *testing.T) {
	f := newFixture(reply("zz"), reply("g1f3"))
	f.decider.illegal = map[string]any{"action": "retry"}
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	if len(f.mover.reqs) != 2 {
		t.Fatalf("proposals = %d, want 2", len(f.mover.reqs))
	}
	if !strings.Contains(f.mover.reqs[1].Feedback, "had no legal move") {
		t.Errorf("retry feedback = %q", f.mover.reqs[1].Feedback)
	}
	opp := g.Moves[len(g.Moves)-1]
	if opp.UCI != "g1f3" || opp.Turn.Attempts != 2 {
		t.Errorf("opponent move = %s attempts %d", opp.UCI, opp.Turn.Attempts)
	}
}

func TestOpponentRandomFallback(t *testing.T) {
	f := newFixture(reply("zz"))
	f.decider.illegal = map[string]any{"action": "random"}
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	opp := g.Moves[len(g.Moves)-1]
	if opp.UCI != testLegal[0] {
		t.Errorf("random move = %s, want %s", opp.UCI, testLegal[0])
	}
	if opp.Turn.Fallback != "random" || opp.Turn.Attempts != 1 {
		t.Errorf("turn = %+v", opp.Turn)
	}
}

func TestOpponentFifthFailureIsRandom(t *testing.T) {
	steps := []moveStep{reply("zz"), reply("zz"), reply("zz"), reply("zz"), reply("zz")}
	f := newFixture(steps...)
	f.decider.illegal = map[string]any{"action": "retry"}
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	if len(f.mover.reqs) != 5 {
		t.Errorf("proposals = %d, want 5", len(f.mover.reqs))
	}
	if n := f.decider.count(DecisionIllegal); n != 4 {
		t.Errorf("illegal decisions = %d, want 4 (none after the fifth failure)", n)
	}
	opp := g.Moves[len(g.Moves)-1]
	if opp.Turn.Attempts != 5 || opp.Turn.Fallback != "random" {
		t.Errorf("turn = %+v", opp.Turn)
	}
}

func TestOpponentResignFallback(t *testing.T) {
	f := newFixture(reply("zz"))
	f.decider.illegal = map[string]any{"action": "resign"}
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	if g.Status != StatusWhiteWon || g.Method != "resignation" {
		t.Errorf("status = %s method = %q, want white_won resignation", g.Status, g.Method)
	}
	if len(g.Moves) != 1 {
		t.Errorf("moves = %d, want only the human move", len(g.Moves))
	}
	if len(g.Legal) != 0 {
		t.Errorf("Legal = %v, want empty for a finished game", g.Legal)
	}
}

func TestDeciderErrorFallsBackToFixed(t *testing.T) {
	f := newFixture(reply("d7d5"))
	f.decider.errs[DecisionPolicy] = errors.New("boom")
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	req := f.mover.reqs[0]
	if req.Persona != "You are a careful chess player." || req.Route != "" {
		t.Errorf("request = route %q persona %q, want the fixed policy", req.Route, req.Persona)
	}
	opp := g.Moves[len(g.Moves)-1]
	if len(opp.Turn.Errors) == 0 || !strings.Contains(strings.Join(opp.Turn.Errors, "|"), "boom") {
		t.Errorf("Turn.Errors = %v, want the decider error", opp.Turn.Errors)
	}
}

func TestChoicePicksHighestScore(t *testing.T) {
	f := newFixture(reply("d2d4", "g1f3", "d7d5"))
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	if opp := g.Moves[len(g.Moves)-1]; opp.UCI != "g1f3" {
		t.Errorf("chosen = %s, want g1f3 (highest captured value)", opp.UCI)
	}
	if n := f.decider.count(DecisionChoice); n != 3 {
		t.Errorf("choice decisions = %d, want 3", n)
	}
}

func TestChoiceTieGoesToLowerRank(t *testing.T) {
	f := newFixture(reply("d2d4", "e2e4"))
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "g1f3")
	if opp := g.Moves[len(g.Moves)-1]; opp.UCI != "d2d4" {
		t.Errorf("chosen = %s, want d2d4 (rank 0 on a tie)", opp.UCI)
	}
}

// rankDecider answers the choice decision with a scripted score per rank. A nil
// entry gives a decision with no "score" key. Other decisions go to base.
type rankDecider struct {
	base   *fakeDecider
	scores []any
}

func (d rankDecider) Decide(ctx context.Context, key string, in map[string]any) (map[string]any, error) {
	if key != DecisionChoice {
		return d.base.Decide(ctx, key, in)
	}
	d.base.calls = append(d.base.calls, key)
	v := d.scores[in["rank"].(int)]
	if v == nil {
		return map[string]any{}, nil
	}
	return map[string]any{"score": v}, nil
}

// scoredFixture is newFixture with the choice decision scripted by rank.
func scoredFixture(scores []any, steps ...moveStep) *fixture {
	f := newFixture(steps...)
	rules := fakeRules{
		legal:    testLegal,
		features: map[string]Features{"g1f3": {CapturedValue: 5}, "d7d5": {CapturedValue: 2}},
	}
	f.svc = NewService(f.store, rules, f.mover, rankDecider{base: f.decider, scores: scores}, Options{
		Now:  func() time.Time { return fixedNow },
		Intn: func(int) int { return 0 },
	})
	return f
}

func TestChoiceRecordsScores(t *testing.T) {
	f := scoredFixture([]any{0.2, 0.7, 0.5, 0.1}, reply("d2d4", "g1f3", "d7d5", "e2e4"))
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	opp := g.Moves[len(g.Moves)-1]
	if opp.UCI != "g1f3" {
		t.Errorf("chosen = %s, want g1f3 (highest score)", opp.UCI)
	}
	want := map[string]float64{"d2d4": 0.2, "g1f3": 0.7, "d7d5": 0.5, "e2e4": 0.1}
	if len(opp.Turn.Scores) != len(want) {
		t.Fatalf("Turn.Scores = %v, want %v", opp.Turn.Scores, want)
	}
	for uci, score := range want {
		if got, ok := opp.Turn.Scores[uci]; !ok || got != score {
			t.Errorf("Turn.Scores[%s] = %v (present %v), want %v", uci, got, ok, score)
		}
	}
}

func TestChoiceScoreWithoutNumberIsLeftOut(t *testing.T) {
	f := scoredFixture([]any{0.4, nil, 0.3}, reply("d2d4", "g1f3", "d7d5"))
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	opp := g.Moves[len(g.Moves)-1]
	if opp.UCI != "d2d4" {
		t.Errorf("chosen = %s, want d2d4 (the only candidate with a score that wins)", opp.UCI)
	}
	if _, ok := opp.Turn.Scores["g1f3"]; ok {
		t.Errorf("Turn.Scores has g1f3 without a numeric score: %v", opp.Turn.Scores)
	}
	if len(opp.Turn.Scores) != 2 || opp.Turn.Scores["d2d4"] != 0.4 || opp.Turn.Scores["d7d5"] != 0.3 {
		t.Errorf("Turn.Scores = %v, want d2d4 and d7d5 only", opp.Turn.Scores)
	}
	if _, err := json.Marshal(opp.Turn); err != nil {
		t.Errorf("turn does not encode: %v", err)
	}
}

func TestSingleCandidateHasNoScores(t *testing.T) {
	f := scoredFixture([]any{0.9}, reply("d7d5"))
	g := mustNewGame(t, f, White)
	g = mustPlay(t, f, g.ID, "e2e4")
	opp := g.Moves[len(g.Moves)-1]
	if opp.UCI != "d7d5" {
		t.Errorf("chosen = %s, want d7d5", opp.UCI)
	}
	if opp.Turn.Scores != nil {
		t.Errorf("Turn.Scores = %v, want nil for a single candidate", opp.Turn.Scores)
	}
	if n := f.decider.count(DecisionChoice); n != 0 {
		t.Errorf("choice decisions = %d, want 0", n)
	}
}

func TestResign(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	g := mustNewGame(t, f, White)
	g, err := f.svc.Resign(ctx, g.ID)
	if err != nil {
		t.Fatalf("Resign: %v", err)
	}
	if g.Status != StatusBlackWon || g.Method != "resignation" {
		t.Errorf("status = %s method = %q, want black_won resignation", g.Status, g.Method)
	}
	if _, err := f.svc.Resign(ctx, g.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("second resign: err = %v, want ErrConflict", err)
	}
	if _, err := f.svc.Resign(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown game: err = %v, want ErrNotFound", err)
	}
}

func TestGamesNeverNil(t *testing.T) {
	f := newFixture()
	games, err := f.svc.Games(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if games == nil {
		t.Error("Games returned nil")
	}
}

func TestFixedDeciderPolicy(t *testing.T) {
	tests := []struct {
		difficulty string
		temp       float64
		candidates int
	}{
		{"easy", 1.0, 1},
		{"normal", 0.6, 3},
		{"hard", 0.2, 5},
	}
	for _, tt := range tests {
		out, err := FixedDecider{}.Decide(context.Background(), DecisionPolicy, map[string]any{"difficulty": tt.difficulty})
		if err != nil {
			t.Fatalf("%s: %v", tt.difficulty, err)
		}
		if out["route"] != "" || out["persona"] != "You are a careful chess player." {
			t.Errorf("%s: route/persona = %v / %v", tt.difficulty, out["route"], out["persona"])
		}
		if out["temperature"] != tt.temp || out["candidates"] != tt.candidates {
			t.Errorf("%s: temperature %v candidates %v, want %v and %v", tt.difficulty, out["temperature"], out["candidates"], tt.temp, tt.candidates)
		}
	}
}

func TestFixedDeciderIllegal(t *testing.T) {
	for _, tt := range []struct {
		attempt any
		want    string
	}{{1, "retry"}, {2.0, "retry"}, {3, "random"}, {float64(4), "random"}} {
		out, err := FixedDecider{}.Decide(context.Background(), DecisionIllegal, map[string]any{"attempt": tt.attempt})
		if err != nil {
			t.Fatal(err)
		}
		if out["action"] != tt.want {
			t.Errorf("attempt %v: action = %v, want %s", tt.attempt, out["action"], tt.want)
		}
	}
}

func TestFixedDeciderChoice(t *testing.T) {
	in := map[string]any{
		"captured_value": 5,
		"check":          true,
		"promotion":      true,
		"attacked":       true,
		"moved_value":    3,
		"rank":           1,
	}
	out, err := FixedDecider{}.Decide(context.Background(), DecisionChoice, in)
	if err != nil {
		t.Fatal(err)
	}
	// 5*10 + 3 (check) + 8 (promotion) - 3*10 (attacked) - 1 (rank) = 30
	if out["score"] != 30.0 {
		t.Errorf("score = %v, want 30", out["score"])
	}
	mate, err := FixedDecider{}.Decide(context.Background(), DecisionChoice, map[string]any{"mate": true})
	if err != nil {
		t.Fatal(err)
	}
	if mate["score"] != 1000.0 {
		t.Errorf("mate score = %v, want 1000", mate["score"])
	}
}

func TestFixedDeciderUnknownKey(t *testing.T) {
	_, err := FixedDecider{}.Decide(context.Background(), "nope", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
