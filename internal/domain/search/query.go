// Package search is the read side's validated query object for US8's unified
// search (contracts/search.md). It knows nothing of PocketBase, the registry
// or the store — only what a request must satisfy before anything is asked.
package search

import (
	"fmt"
	"slices"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// MinTermLen and MaxTermLen bound `q` (contracts/search.md §1).
const (
	MinTermLen = 1
	MaxTermLen = 200
)

// Query is one validated /search request. Term is never logged, never
// echoed and never carried in an error message: FR-075 and research D-12
// treat it as a first-class secret, and the only safe place for it is this
// struct and the store query built from it.
type Query struct {
	Term      string
	PatientID string
	// Kinds is never empty once NewQuery has built one: an absent `?kinds=`
	// resolves to every kind `registered` named, in that order, so a caller
	// never has to special-case "every kind" downstream.
	Kinds []kind.Kind
}

// NewQuery validates q, patient and kinds.
//
// registered is the kinds this build actually serves search over, in the
// order groups are meant to render — the record registry's own Kinds(),
// handed in because the domain layer does not know the registry exists.
//
// patientID absence is refused here too, even though the HTTP edge already
// refuses it earlier with its own 400 patient_required (contracts/search.md
// §3, FR-070): this is the defence a caller that skipped the edge still
// gets, and it is why the check exists here rather than being trusted to
// have already happened.
func NewQuery(term, patientID string, kindValues []string, registered []kind.Kind) (Query, error) {
	var invalid domain.ValidationError

	switch {
	case len(term) < MinTermLen:
		invalid.Add("q", domain.CodeRequired, "a search term is required")
	case len(term) > MaxTermLen:
		invalid.Addf("q", domain.CodeTooLong, "a search term is at most %d characters", MaxTermLen)
	}

	if patientID == "" {
		invalid.Add("patient", domain.CodeRequired, "a patient is required")
	}

	kinds, err := resolveKinds(kindValues, registered)
	if err != nil {
		return Query{}, err
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	return Query{Term: term, PatientID: patientID, Kinds: kinds}, nil
}

// resolveKinds turns `?kinds=` — a csv of path segments (contracts/search.md
// §1, the same spelling `?kind=` takes on the cross-kind records list) —
// into the registered set it names, or every registered kind when the
// caller named none. A segment naming an undeclared or unregistered kind is
// domain.ErrBadRequest and never echoes the value it did not recognise
// (contracts/search.md §3): naming it back would tell an anonymous prober
// which kinds this build does and does not serve, one guess at a time.
func resolveKinds(segments []string, registered []kind.Kind) ([]kind.Kind, error) {
	if len(segments) == 0 {
		return slices.Clone(registered), nil
	}

	resolved := make([]kind.Kind, 0, len(segments))

	for _, segment := range segments {
		k, ok := kind.FromSegment(segment)
		if !ok || !slices.Contains(registered, k) {
			return nil, fmt.Errorf("%w: a narrowing named a kind this instance does not serve search over", domain.ErrBadRequest)
		}

		resolved = append(resolved, k)
	}

	return resolved, nil
}
