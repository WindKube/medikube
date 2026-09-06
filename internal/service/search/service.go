package search

import (
	"context"
	"errors"
	"fmt"

	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	domainsearch "medikube/internal/domain/search"
)

// The two empty_reason values contracts/search.md publishes. The zero value
// (empty string) is the third: a search that found something.
const (
	EmptyReasonNoMatches = "no_matches"
	EmptyReasonNoRecords = "no_records"
)

var (
	// ErrNoSearcher and ErrNoCounter guard against a zero-value Service.
	ErrNoSearcher = errors.New("search: no searcher")
	ErrNoCounter  = errors.New("search: no counter")

	// ErrNoAuthorizer guards the same way.
	ErrNoAuthorizer = errors.New("search: no authorizer")
)

// Authorizer is the checkpoint a search is reached through — the same shape
// records.Authorizer takes, declared again here for the same reason that
// package declares its own: this package does not import internal/records
// (records already imports this one).
type Authorizer interface {
	Patient(ctx context.Context, actor access.Actor, patientID string, need access.Permission) (access.Grant, error)
}

// Group is one kind's page of a grouped search result.
type Group struct {
	Kind       kind.Kind
	Items      []Hit
	NextCursor *string
	HasMore    bool
}

// Result is a whole grouped search: contracts/search.md §2. Only kinds with
// at least one match appear in Groups (FR-072).
type Result struct {
	Groups []Group
	// EmptyReason is "" when Groups is non-empty, "no_matches" when it is
	// empty but the patient has other indexed rows, and "no_records" when
	// the patient has none at all (US8 scenario 2).
	EmptyReason string
}

// Service is US8's read side. The write side (internal/service/search's
// Indexer) is a different type on purpose: nothing that only ever reads the
// index needs the ability to write it.
type Service struct {
	searcher   Searcher
	counter    Counter
	authorizer Authorizer
}

func NewService(searcher Searcher, counter Counter, authorizer Authorizer) (*Service, error) {
	if searcher == nil {
		return nil, ErrNoSearcher
	}

	if counter == nil {
		return nil, ErrNoCounter
	}

	if authorizer == nil {
		return nil, ErrNoAuthorizer
	}

	return &Service{searcher: searcher, counter: counter, authorizer: authorizer}, nil
}

// Cursors is one incoming request's per-group continuation tokens, keyed by
// kind (contracts/search.md §1: `cursor` is a csv of `kind:cursor` pairs,
// one cursor per group).
type Cursors map[kind.Kind]string

// Search runs one grouped search. The patient is authorized once, before any
// group is read (contracts/search.md §4): the index is patient-scoped, so a
// term matching only another account's rows answers exactly like a term
// matching nothing at all, and this is the one call that could otherwise
// tell the two apart.
func (s *Service) Search(
	ctx context.Context, actor access.Actor, query domainsearch.Query, limit int, cursors Cursors,
) (Result, error) {
	if _, err := s.authorizer.Patient(ctx, actor, query.PatientID, access.PermView); err != nil {
		return Result{}, err
	}

	groups := make([]Group, 0, len(query.Kinds))

	for _, k := range query.Kinds {
		page, err := s.searcher.SearchKind(ctx, query.PatientID, k, query.Term, limit, cursors[k])
		if err != nil {
			return Result{}, fmt.Errorf("search: %s: %w", k, err)
		}

		if len(page.Items) == 0 {
			continue
		}

		groups = append(groups, Group{
			Kind:       k,
			Items:      page.Items,
			NextCursor: page.NextCursor,
			HasMore:    page.NextCursor != nil,
		})
	}

	if len(groups) > 0 {
		return Result{Groups: groups}, nil
	}

	// Every group came back empty. Which of the two empty states this is
	// does not depend on the term at all — it depends on whether the patient
	// has anything indexed, across every kind, term aside.
	total, err := s.counter.Count(ctx, query.PatientID, nil)
	if err != nil {
		return Result{}, fmt.Errorf("search: counting the patient's index: %w", err)
	}

	if total == 0 {
		return Result{EmptyReason: EmptyReasonNoRecords}, nil
	}

	return Result{EmptyReason: EmptyReasonNoMatches}, nil
}
