// Package random implements core.Mover by picking legal moves at random.
package random

import (
	"context"
	"math/rand"
	"sync"

	"chess/internal/core"
)

// Mover picks moves at random from the legal moves it is given.
type Mover struct {
	mu sync.Mutex
	r  *rand.Rand
}

var _ core.Mover = (*Mover)(nil)

// New returns a Mover whose choices are fully determined by seed.
func New(seed int64) *Mover {
	return &Mover{r: rand.New(rand.NewSource(seed))}
}

// Propose returns min(Count, len(Legal)) distinct entries of req.Legal in random order.
func (m *Mover) Propose(ctx context.Context, req core.MoveRequest) (core.MoveReply, error) {
	if err := ctx.Err(); err != nil {
		return core.MoveReply{}, err
	}
	n := min(req.Count, len(req.Legal))
	if n < 0 {
		n = 0
	}
	m.mu.Lock()
	perm := m.r.Perm(len(req.Legal))
	m.mu.Unlock()

	cands := make([]string, 0, n)
	for _, i := range perm[:n] {
		cands = append(cands, req.Legal[i])
	}
	return core.MoveReply{
		Candidates: cands,
		Route:      "random",
		Backend:    "random",
		Model:      "random",
	}, nil
}
