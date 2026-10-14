package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"bookmarks/internal/bookmark"
)

func TestAddThenList(t *testing.T) {
	ctx := context.Background()
	s := New()

	a, err := s.Add(ctx, "Go", "https://go.dev")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	b, err := s.Add(ctx, "Example", "http://example.com")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if a.ID != 1 || b.ID != 2 {
		t.Fatalf("IDs = %d, %d; want 1, 2", a.ID, b.ID)
	}
	if a.Title != "Go" || a.URL != "https://go.dev" {
		t.Fatalf("returned bookmark = %+v", a)
	}
	if a.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(List) = %d; want 2", len(list))
	}
	if list[0].ID != 1 || list[1].ID != 2 {
		t.Fatalf("List order = %d, %d; want 1, 2", list[0].ID, list[1].ID)
	}
	if list[0].Title != "Go" || list[1].URL != "http://example.com" {
		t.Fatalf("List contents = %+v", list)
	}
}

func TestAddValidation(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		title string
		url   string
	}{
		{"empty title", "", "https://go.dev"},
		{"whitespace-only title", "   \t ", "https://go.dev"},
		{"title over 200 runes", strings.Repeat("a", 201), "https://go.dev"},
		{"title over 200 runes multibyte", strings.Repeat("é", 201), "https://go.dev"},
		{"url without scheme", "Go", "go.dev"},
		{"ftp scheme", "Go", "ftp://go.dev"},
		{"javascript scheme", "Go", "javascript:alert(1)"},
		{"http without host", "Go", "http://"},
		{"empty url", "Go", ""},
		{"unparseable url", "Go", "http://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New()
			_, err := s.Add(ctx, tt.title, tt.url)
			if !errors.Is(err, bookmark.ErrInvalid) {
				t.Fatalf("err = %v; want ErrInvalid", err)
			}
			list, _ := s.List(ctx)
			if len(list) != 0 {
				t.Fatalf("invalid add stored a bookmark: %+v", list)
			}
		})
	}
}

func TestTitleLimitCountsRunesAfterTrim(t *testing.T) {
	ctx := context.Background()
	s := New()

	// 200 multibyte runes is valid even though its byte length is over 200.
	ok := strings.Repeat("é", 200)
	if _, err := s.Add(ctx, ok, "https://go.dev"); err != nil {
		t.Fatalf("200-rune title rejected: %v", err)
	}

	// Surrounding whitespace does not count toward the limit.
	padded := "  " + strings.Repeat("a", 200) + "  "
	b, err := s.Add(ctx, padded, "https://go.dev")
	if err != nil {
		t.Fatalf("padded 200-rune title rejected: %v", err)
	}
	if b.Title != strings.Repeat("a", 200) {
		t.Fatalf("title not trimmed to 200 runes: len %d", len(b.Title))
	}
}

func TestAddTrimsWhitespace(t *testing.T) {
	ctx := context.Background()
	s := New()

	b, err := s.Add(ctx, "  Go  ", "\t https://go.dev \n")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if b.Title != "Go" {
		t.Fatalf("Title = %q; want %q", b.Title, "Go")
	}
	if b.URL != "https://go.dev" {
		t.Fatalf("URL = %q; want %q", b.URL, "https://go.dev")
	}

	list, _ := s.List(ctx)
	if len(list) != 1 || list[0].Title != "Go" || list[0].URL != "https://go.dev" {
		t.Fatalf("stored = %+v; want trimmed values", list)
	}
}

func TestDeleteRemoves(t *testing.T) {
	ctx := context.Background()
	s := New()
	s.Add(ctx, "A", "https://a.example")
	s.Add(ctx, "B", "https://b.example")
	s.Add(ctx, "C", "https://c.example")

	if err := s.Delete(ctx, 2); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, _ := s.List(ctx)
	if len(list) != 2 || list[0].ID != 1 || list[1].ID != 3 {
		t.Fatalf("after delete, List = %+v; want IDs 1, 3", list)
	}
}

func TestDeleteUnknownID(t *testing.T) {
	ctx := context.Background()
	s := New()
	s.Add(ctx, "A", "https://a.example")

	err := s.Delete(ctx, 99)
	if !errors.Is(err, bookmark.ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
	if err := s.Delete(ctx, 1); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if err := s.Delete(ctx, 1); !errors.Is(err, bookmark.ErrNotFound) {
		t.Fatalf("second delete err = %v; want ErrNotFound", err)
	}
}

func TestIDNotReusedAfterDelete(t *testing.T) {
	ctx := context.Background()
	s := New()
	s.Add(ctx, "A", "https://a.example")
	s.Add(ctx, "B", "https://b.example")
	if err := s.Delete(ctx, 2); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	c, err := s.Add(ctx, "C", "https://c.example")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if c.ID != 3 {
		t.Fatalf("ID after delete = %d; want 3", c.ID)
	}
}

func TestEmptyListNonNil(t *testing.T) {
	ctx := context.Background()
	s := New()

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil {
		t.Fatal("List returned nil; want empty non-nil slice")
	}
	if len(list) != 0 {
		t.Fatalf("len = %d; want 0", len(list))
	}
}
