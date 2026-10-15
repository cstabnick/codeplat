package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"decide/internal/app"
	"decide/internal/apptest"
	"decide/internal/config"
	"decide/internal/core"
)

func TestExampleShipping(t *testing.T) {
	svc := apptest.New(t)
	ctx := context.Background()
	tests := []struct {
		name  string
		input map[string]any
		want  float64
	}{
		{"US light", map[string]any{"country": "US", "weight": 3}, 5},
		{"US heavy", map[string]any{"country": "US", "weight": 10}, 12},
		{"FR light", map[string]any{"country": "FR", "weight": 2}, 15},
		{"FR heavy", map[string]any{"country": "FR", "weight": 9}, 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := svc.Evaluate(ctx, "example-shipping", tc.input)
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			// The cells are evaluated by zen.Lang.Evaluate, so cost is a number.
			got, ok := out["cost"].(float64)
			if !ok {
				t.Fatalf("cost = %#v, want a number", out["cost"])
			}
			if got != tc.want {
				t.Errorf("cost = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestModelsIncludesExample(t *testing.T) {
	svc := apptest.New(t)
	keys, err := svc.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if !slices.Contains(keys, "example-shipping") {
		t.Errorf("Models = %v, want it to include example-shipping", keys)
	}
}

func TestUnknownModelIsNotFound(t *testing.T) {
	svc := apptest.New(t)
	_, err := svc.Evaluate(context.Background(), "no-such-model", nil)
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want it to wrap core.ErrNotFound", err)
	}
}

func TestNewRejectsBadModelsDir(t *testing.T) {
	tests := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"missing", filepath.Join(t.TempDir(), "missing")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := app.New(config.Config{ModelsDir: tc.dir})
			if err == nil {
				t.Errorf("New returned %v, want an error", svc)
			}
		})
	}
}
