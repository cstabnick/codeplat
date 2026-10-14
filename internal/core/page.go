package core

import "fmt"

const (
	// DefaultPageLimit is the limit used when a Page has Limit 0.
	DefaultPageLimit = 50
	// MaxPageLimit is the largest limit a list call accepts.
	MaxPageLimit = 200
)

// Page selects one window of a list. Limit is how many items to return;
// Offset is how many matching items to skip. The JSON names are "limit" and
// "offset", so a struct that embeds Page decodes them from request bodies.
type Page struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// resolve returns p with the default limit filled in. It fails with
// ErrInvalid if the limit is outside 1 to MaxPageLimit (after the default)
// or the offset is negative.
func (p Page) resolve() (Page, error) {
	if p.Limit == 0 {
		p.Limit = DefaultPageLimit
	}
	if p.Limit < 1 || p.Limit > MaxPageLimit {
		return Page{}, invalid(fmt.Sprintf("limit must be between 1 and %d", MaxPageLimit))
	}
	if p.Offset < 0 {
		return Page{}, invalid("offset cannot be negative")
	}
	return p, nil
}
