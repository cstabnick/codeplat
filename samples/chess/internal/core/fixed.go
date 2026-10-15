package core

import (
	"context"
	"fmt"
)

// FixedDecider answers every decision with a fixed rule set. The Service falls
// back to it whenever a real Decider fails.
type FixedDecider struct{}

func (FixedDecider) Decide(ctx context.Context, key string, input map[string]any) (map[string]any, error) {
	switch key {
	case DecisionPolicy:
		return fixedPolicy(input), nil
	case DecisionIllegal:
		attempt, _ := numberOf(input["attempt"])
		if attempt < 3 {
			return map[string]any{"action": "retry"}, nil
		}
		return map[string]any{"action": "random"}, nil
	case DecisionChoice:
		return map[string]any{"score": fixedScore(input)}, nil
	}
	return nil, fmt.Errorf("fixed decider: unknown decision %q: %w", key, ErrNotFound)
}

func fixedPolicy(input map[string]any) map[string]any {
	difficulty, _ := input["difficulty"].(string)
	temperature, candidates := 0.6, 3
	switch difficulty {
	case "easy":
		temperature, candidates = 1.0, 1
	case "hard":
		temperature, candidates = 0.2, 5
	}
	return map[string]any{
		"route":       "",
		"temperature": temperature,
		"persona":     "You are a careful chess player.",
		"candidates":  candidates,
	}
}

func fixedScore(in map[string]any) float64 {
	captured, _ := numberOf(in["captured_value"])
	moved, _ := numberOf(in["moved_value"])
	rank, _ := numberOf(in["rank"])
	score := captured*10 - rank
	if flag(in, "check") {
		score += 3
	}
	if flag(in, "mate") {
		score += 1000
	}
	if flag(in, "promotion") {
		score += 8
	}
	if flag(in, "attacked") {
		score -= moved * 10
	}
	return score
}

func flag(in map[string]any, key string) bool {
	b, _ := in[key].(bool)
	return b
}
