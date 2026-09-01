package domain

import (
	"errors"
	"strings"
)

// Page is the envelope every list in the API returns, without exception. One
// shape everywhere is what stopped upstream's search from paging incorrectly:
// a per-group limit under a single global "has more" cannot be right.
type Page[T any] struct {
	Items []T `json:"items"`
	// Opaque and HMAC-signed, encoding the sort keys and the last id — never an
	// offset, so a concurrent insert cannot duplicate or skip a row. nil is the
	// last page, and the member is present either way.
	NextCursor *string `json:"next_cursor"`
	// Only when ?count=true was passed. Counting a large owner's records on
	// every page is a cost nobody asked for.
	Total *int `json:"total,omitempty"`
}

// NewPage takes a copy, so a caller that keeps building in its slice cannot
// change a page it has already handed on, and substitutes an empty slice for a
// nil one: the contract says a list marshals as [] and never null.
func NewPage[T any](items []T, nextCursor *string) Page[T] {
	copied := make([]T, len(items))
	copy(copied, items)
	return Page[T]{Items: copied, NextCursor: nextCursor}
}

func (p Page[T]) WithTotal(total int) Page[T] {
	p.Total = &total
	return p
}

var errNotASortKey = errors.New("domain: not a sort key")

// SortKey is one term of a keyset ordering: the field, and whether it descends.
// The cursor encodes these, so the spelling has to be exact — a key that
// renders differently from the way it parsed produces a cursor that decodes
// into a different query.
type SortKey struct {
	Field string
	Desc  bool
}

func (k SortKey) String() string {
	if k.Desc {
		return "-" + k.Field
	}
	return k.Field
}

// ParseSortKey reads one term of the `?sort=` list. Whether the field is one a
// resource actually allows is the edge's question, answered against that
// resource's allowlist; a value outside it is 422 and never silently ignored.
func ParseSortKey(term string) (SortKey, error) {
	key := SortKey{Field: strings.TrimPrefix(term, "-"), Desc: strings.HasPrefix(term, "-")}
	// A sort field is a column name, and every column MediKube declares is
	// lower snake_case. Anything else is a second `-`, a stray space or an
	// injection attempt, and none of them is a field this can go on to name.
	if key.Field == "" || strings.ContainsFunc(key.Field, notColumnRune) {
		return SortKey{}, errNotASortKey
	}
	return key, nil
}

func notColumnRune(r rune) bool {
	return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_'
}
