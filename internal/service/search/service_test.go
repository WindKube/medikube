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
}

func newFakeSearcher() *fakeSearcher {
	return &fakeSearcher{pages: map[kind.Kind][]domain.Page[Hit]{}, calls: map[kind.Kind]int{}}
}

func (f *fakeSearcher) SearchKind(
	_ context.Context, _ string, k kind.Kind, _ string, _ int, _ string,
) (domain.Page[Hit], error) {
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

func TestNewServiceRefusesMissingDependencies(t *testing.T) {
	t.Parallel()

	searcher, counter, authorizer := newFakeSearcher(), &fakeCounter{}, &fakeAuthorizer{}

	_, err := NewService(nil, counter, authorizer)
	require.ErrorIs(t, err, ErrNoSearcher)

	_, err = NewService(searcher, nil, authorizer)
	require.ErrorIs(t, err, ErrNoCounter)

	_, err = NewService(searcher, counter, nil)
	require.ErrorIs(t, err, ErrNoAuthorizer)
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

	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{})
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
		svc, err := NewService(searcher, &fakeCounter{total: 3}, &fakeAuthorizer{})
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
		svc, err := NewService(searcher, &fakeCounter{total: 0}, &fakeAuthorizer{})
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
	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{err: refused})
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

	svc, err := NewService(searcher, &fakeCounter{}, &fakeAuthorizer{})
	require.NoError(t, err)

	query := domainsearch.Query{Term: "warfarin", PatientID: "pat1", Kinds: []kind.Kind{kind.Medication}}

	_, err = svc.Search(context.Background(), access.Actor{}, query, 25, Cursors{kind.Medication: "a-cursor"})
	require.NoError(t, err)
	assert.Equal(t, 1, searcher.calls[kind.Medication])
}
