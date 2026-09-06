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

// MatchAny and MatchAll are `?match=` (contracts/search.md §1): whether
// `?tags=` narrows to a record carrying at least one of the named tags or
// every one of them. MatchAny is the default. Declared here rather than
// imported from internal/records — that package is on the PocketBase side of
// the [PB] boundary and this one is not.
const (
	MatchAny = "any"
	MatchAll = "all"
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
	// TagIDs is `?tags=`, unvalidated against the actor's own tags: that
	// check needs the tag service, which this package does not import, so it
	// is the service layer's job (contracts/search.md §5, T164-T177 follow-up).
	TagIDs []string
	// Match is `?match=`, MatchAny or MatchAll, defaulting to MatchAny.
	Match string
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
//
// tagIDs is `?tags=`, structurally only: whether each id actually belongs to
// the actor is the service layer's job, not this package's, because that
// check needs the tag service. match is `?match=`, defaulting to MatchAny
// when empty; anything else is 400 bad_request.
func NewQuery(term, patientID string, kindValues, tagIDs []string, match string, registered []kind.Kind) (Query, error) {
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

	resolvedMatch, err := resolveMatch(match)
	if err != nil {
		return Query{}, err
	}

	if err := invalid.OrNil(); err != nil {
		return Query{}, err
	}

	return Query{Term: term, PatientID: patientID, Kinds: kinds, TagIDs: tagIDs, Match: resolvedMatch}, nil
}

// resolveMatch defaults an absent `?match=` to MatchAny and refuses anything
// that is neither spelling.
func resolveMatch(match string) (string, error) {
	switch match {
	case "":
		return MatchAny, nil
	case MatchAny, MatchAll:
		return match, nil
	default:
		return "", fmt.Errorf("%w: match must be %q or %q", domain.ErrBadRequest, MatchAny, MatchAll)
	}
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
