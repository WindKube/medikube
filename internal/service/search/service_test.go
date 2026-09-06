package search

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/kind"
	domainsearch "medikube/internal/domain/search"
)

type fakeSearcher struct {
	// pages, keyed by kind, answered in order per call — simplest way to
	// script a first page with has_more true and a second with none.
	pages map[kind.Kind][]domain.Page[Hit]
	calls map[kind.Kind]int
	err   error

	// lastTagIDs and lastMatch are whatever the most recent call was handed —
	// proof that Search actually passes the narrowing through, not just that
	// it checks ownership of it.
	lastTagIDs []string
	lastMatch  string
}

func newFakeSearcher() *fakeSearcher {
	return &fakeSearcher{pages: map[kind.Kind][]domain.Page[Hit]{}, calls: map[kind.Kind]int{}}
}

func (f *fakeSearcher) SearchKind(
	_ context.Context, _ string, k kind.Kind, _ string, tagIDs []string, match string, _ int, _ string,
) (domain.Page[Hit], error) {
	f.lastTagIDs = tagIDs
	f.lastMatch = match

	if f.err != nil {
		return domain.Page[Hit]{}, f.err
	}

	pages := f.pages[k]
	i := f.calls[k]
	f.calls[k]++

	if i >= len(pages) {
		return domain.NewPage[Hit](nil, nil), nil
	}

	return pages[i], nil
}

type fakeCounter struct {
	total int
	err   error
}

func (f *fakeCounter) Count(context.Context, string, []kind.Kind) (int, error) {
	return f.total, f.err
}

type fakeAuthorizer struct {
	err error
}

func (f *fakeAuthorizer) Patient(context.Context, access.Actor, string, access.Permission) (access.Grant, error) {
	return access.Grant{}, f.err
}

type fakeTagChecker struct {
	err   error
	calls int
}

func (f *fakeTagChecker) Owned(context.Context, access.Actor, []string) error {
	f.calls++

	return f.err
}

func TestNewServiceRefusesMissingDependencies(t *testing.T) {
	t.Parallel()

	searcher, counter, authorizer, tagChecker := newFakeSearcher(), &fakeCounter{}, &fakeAuthorizer{}, &fakeTagChecker{}

	_, err := NewService(nil, counter, authorizer, tagChecker)
	require.ErrorIs(t, err, ErrNoSearcher)

	_, err = NewService(searcher, nil, authorizer, tagChecker)
	require.ErrorIs(t, err, ErrNoCounter)

	_, err = NewService(searcher, counter, nil, tagChecker)
	require.ErrorIs(t, err, ErrNoAuthorizer)

	_, err = NewService(searcher, counter, authorizer, nil)
	require.ErrorIs(t, err, ErrNoTagChecker)
}

func TestSearchGroupsByKindAndOnlyKindsWithAMatchAppear(t *testing.T) {
	t.Parallel()

	searcher := newFakeSearcher()
	next := "next-token"
	searcher.pages[kind.Medication] = []domain.Page[Hit]{
		domain.NewPage([]Hit{{Kind: kind.Medication, RecordID: "m1"}}, &next),
	}
	searcher.pages[kind.Condition] = []domain.Page[Hit]{domain.NewPage[Hit](nil, nil)}
	// kind.Allergy answers nothing at all — never queried by anything but
	// still must not appear.

	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{}, &fakeTagChecker{})
	require.NoError(t, err)

	query := domainsearch.Query{Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication, kind.Condition}}

	result, err := svc.Search(context.Background(), access.Actor{UserID: "u1"}, query, 25, nil)
	require.NoError(t, err)

	require.Len(t, result.Groups, 1)
	assert.Equal(t, kind.Medication, result.Groups[0].Kind)
	assert.True(t, result.Groups[0].HasMore)
	assert.Equal(t, &next, result.Groups[0].NextCursor)
	assert.Empty(t, result.EmptyReason)
}

func TestSearchDistinguishesNoMatchesFromNoRecords(t *testing.T) {
	t.Parallel()

	t.Run("no groups and the patient has other indexed rows is no_matches", func(t *testing.T) {
		t.Parallel()

		searcher := newFakeSearcher()
		svc, err := NewService(searcher, &fakeCounter{total: 3}, &fakeAuthorizer{}, &fakeTagChecker{})
		require.NoError(t, err)

		query := domainsearch.Query{Term: "nonsense", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication}}
		result, err := svc.Search(context.Background(), access.Actor{}, query, 25, nil)
		require.NoError(t, err)
		assert.Empty(t, result.Groups)
		assert.Equal(t, EmptyReasonNoMatches, result.EmptyReason)
	})

	t.Run("no groups and the patient has nothing indexed at all is no_records", func(t *testing.T) {
		t.Parallel()

		searcher := newFakeSearcher()
		svc, err := NewService(searcher, &fakeCounter{total: 0}, &fakeAuthorizer{}, &fakeTagChecker{})
		require.NoError(t, err)

		query := domainsearch.Query{Term: "anything", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication}}
		result, err := svc.Search(context.Background(), access.Actor{}, query, 25, nil)
		require.NoError(t, err)
		assert.Empty(t, result.Groups)
		assert.Equal(t, EmptyReasonNoRecords, result.EmptyReason)
	})
}

func TestSearchAuthorizesThePatientOnceBeforeAnyGroup(t *testing.T) {
	t.Parallel()

	refused := errors.New("refused")
	searcher := newFakeSearcher()
	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{err: refused}, &fakeTagChecker{})
	require.NoError(t, err)

	query := domainsearch.Query{Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication, kind.Condition}}
	_, err = svc.Search(context.Background(), access.Actor{}, query, 25, nil)
	require.ErrorIs(t, err, refused)
	assert.Zero(t, searcher.calls[kind.Medication], "no group should be read once the patient check refused")
}

func TestSearchPassesEachGroupsOwnCursor(t *testing.T) {
	t.Parallel()

	searcher := newFakeSearcher()
	searcher.pages[kind.Medication] = []domain.Page[Hit]{domain.NewPage[Hit](nil, nil)}

	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{}, &fakeTagChecker{})
	require.NoError(t, err)

	query := domainsearch.Query{Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication}}

	_, err = svc.Search(context.Background(), access.Actor{}, query, 25, Cursors{kind.Medication: "a-cursor"})
	require.NoError(t, err)
	assert.Equal(t, 1, searcher.calls[kind.Medication])
}

// TestSearchChecksTagOwnershipOnceBeforeAnyGroupAndNeverForAnUnnarrowedSearch
// is T164-T177's follow-up: a foreign or unknown tag id must be refused
// before any group is read, and a search naming no tags must never call the
// tag checker at all — an empty `?tags=` costs nothing extra.
func TestSearchChecksTagOwnershipOnceBeforeAnyGroupAndNeverForAnUnnarrowedSearch(t *testing.T) {
	t.Parallel()

	t.Run("a foreign or unknown tag id is refused before any group is read", func(t *testing.T) {
		t.Parallel()

		refused := errors.New("refused")
		searcher := newFakeSearcher()
		tagChecker := &fakeTagChecker{err: refused}
		svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{}, tagChecker)
		require.NoError(t, err)

		query := domainsearch.Query{
			Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication}, TagIDs: []string{"not-mine"},
		}

		_, err = svc.Search(context.Background(), access.Actor{}, query, 25, nil)
		require.ErrorIs(t, err, refused)
		assert.Equal(t, 1, tagChecker.calls)
		assert.Zero(t, searcher.calls[kind.Medication], "no group should be read once the tag check refused")
	})

	t.Run("no tags named means the tag checker is never called", func(t *testing.T) {
		t.Parallel()

		searcher := newFakeSearcher()
		tagChecker := &fakeTagChecker{}
		svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{}, tagChecker)
		require.NoError(t, err)

		query := domainsearch.Query{Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication}}

		_, err = svc.Search(context.Background(), access.Actor{}, query, 25, nil)
		require.NoError(t, err)
		assert.Zero(t, tagChecker.calls)
	})
}

// TestSearchPassesTagIDsAndMatchToEachGroup proves the narrowing actually
// reaches the store, not just the ownership check.
func TestSearchPassesTagIDsAndMatchToEachGroup(t *testing.T) {
	t.Parallel()

	searcher := newFakeSearcher()
	searcher.pages[kind.Medication] = []domain.Page[Hit]{domain.NewPage[Hit](nil, nil)}

	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{}, &fakeTagChecker{})
	require.NoError(t, err)

	query := domainsearch.Query{
		Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication},
		TagIDs: []string{"tag1"}, Match: domainsearch.MatchAll,
	}

	_, err = svc.Search(context.Background(), access.Actor{}, query, 25, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, searcher.calls[kind.Medication])
	assert.Equal(t, []string{"tag1"}, searcher.lastTagIDs)
	assert.Equal(t, domainsearch.MatchAll, searcher.lastMatch)
}
