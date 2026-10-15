// Package core holds the decision service: it looks up models and evaluates them.
package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"decide/internal/jdm"
)

var (
	// ErrNotFound is returned when a model does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInvalid is returned for a request that breaks a rule.
	ErrInvalid = errors.New("invalid")
)

// KeyPattern is what a model key must match.
var KeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ModelSource loads decision models by key. It is implemented outside core.
type ModelSource interface {
	Get(ctx context.Context, key string) (*jdm.Graph, error) // ErrNotFound when missing
	List(ctx context.Context) ([]string, error)              // sorted keys
}

// Engine evaluates a decision model against an input. It is implemented outside core.
type Engine interface {
	Evaluate(ctx context.Context, g *jdm.Graph, input map[string]any) (map[string]any, error)
}

// Service evaluates decision models.
type Service struct {
	models ModelSource
	engine Engine
}

// NewService returns a Service that loads models from models and evaluates them with engine.
func NewService(models ModelSource, engine Engine) *Service {
	return &Service{models: models, engine: engine}
}

// Evaluate runs the model with the given key on input. A key that does not match
// KeyPattern returns an error wrapping ErrInvalid. Errors from the model source and
// the engine are returned unchanged. A nil input is treated as empty, and a nil
// result is returned as an empty map.
func (s *Service) Evaluate(ctx context.Context, key string, input map[string]any) (map[string]any, error) {
	if !KeyPattern.MatchString(key) {
		return nil, fmt.Errorf("%w: model key %q", ErrInvalid, key)
	}
	g, err := s.models.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if input == nil {
		input = map[string]any{}
	}
	out, err := s.engine.Evaluate(ctx, g, input)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// Models returns the sorted keys of all models. It never returns nil.
func (s *Service) Models(ctx context.Context) ([]string, error) {
	keys, err := s.models.List(ctx)
	if err != nil {
		return nil, err
	}
	if keys == nil {
		keys = []string{}
	}
	return keys, nil
}
