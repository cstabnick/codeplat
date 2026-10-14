package auth

import (
	"errors"
	"testing"

	"codeplat/internal/core"
)

func TestOpenAdmitsEverything(t *testing.T) {
	for _, token := range []string{"", "anything"} {
		p, err := Open{}.Authenticate(token)
		if err != nil {
			t.Fatalf("Open.Authenticate(%q) error = %v, want nil", token, err)
		}
		if p.Name != "" {
			t.Errorf("Open.Authenticate(%q) Name = %q, want empty", token, p.Name)
		}
	}
}

func TestStaticAdmitsKnownTokens(t *testing.T) {
	s, err := NewStatic(map[string]string{"alice": "a1", "bob": "b1"})
	if err != nil {
		t.Fatalf("NewStatic error = %v", err)
	}
	for token, want := range map[string]string{"a1": "alice", "b1": "bob"} {
		p, err := s.Authenticate(token)
		if err != nil {
			t.Fatalf("Authenticate(%q) error = %v, want nil", token, err)
		}
		if p.Name != want {
			t.Errorf("Authenticate(%q) Name = %q, want %q", token, p.Name, want)
		}
	}
}

func TestStaticRejectsUnknownTokens(t *testing.T) {
	s, err := NewStatic(map[string]string{"alice": "a1", "bob": "b1"})
	if err != nil {
		t.Fatalf("NewStatic error = %v", err)
	}
	for _, token := range []string{"", "a", "a10", "x"} {
		p, err := s.Authenticate(token)
		if !errors.Is(err, core.ErrUnauthenticated) {
			t.Errorf("Authenticate(%q) error = %v, want one wrapping core.ErrUnauthenticated", token, err)
		}
		if p.Name != "" {
			t.Errorf("Authenticate(%q) Name = %q, want empty", token, p.Name)
		}
	}
}

func TestNewStaticRejectsBadEntries(t *testing.T) {
	cases := map[string]map[string]string{
		"empty name":       {"": "t1"},
		"name with space":  {"a b": "t1"},
		"empty token":      {"alice": ""},
		"duplicate tokens": {"alice": "same", "bob": "same"},
	}
	for label, tokens := range cases {
		t.Run(label, func(t *testing.T) {
			s, err := NewStatic(tokens)
			if err == nil {
				t.Fatalf("NewStatic(%v) error = nil, want error", tokens)
			}
			if s != nil {
				t.Errorf("NewStatic(%v) returned non-nil authenticator with error", tokens)
			}
		})
	}
}

func TestNewStaticEmptyMapAdmitsNothing(t *testing.T) {
	s, err := NewStatic(map[string]string{})
	if err != nil {
		t.Fatalf("NewStatic(empty) error = %v, want nil", err)
	}
	if _, err := s.Authenticate("x"); !errors.Is(err, core.ErrUnauthenticated) {
		t.Errorf("Authenticate(\"x\") error = %v, want one wrapping core.ErrUnauthenticated", err)
	}
}
