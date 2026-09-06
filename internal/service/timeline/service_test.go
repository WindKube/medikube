package timeline_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/timeline"
)

const (
	kindOne kind.Kind = "fake_kind_one"
	kindTwo kind.Kind = "fake_kind_two"
)

var owner = access.Actor{UserID: recordstest.OwnerID}

func newRegistry(t *testing.T) *records.Registry {
	t.Helper()

	registry := records.NewRegistry()

	require.NoError(t, registry.RegisterSynthetic(
		recordstest.Registration(kindOne, audit.TargetKindLabResult), "fake-kind-ones", "fake_kind_ones"))
	require.NoError(t, registry.RegisterSynthetic(
		recordstest.Registration(kindTwo, audit.TargetKindLabResult), "fake-kind-twos", "fake_kind_twos"))

	return registry
}

func seed(t *testing.T, registry *records.Registry, k kind.Kind, name string) string {
	t.Helper()

	entry, found := registry.FromKind(k)
	require.True(t, found)

	created, err := entry.Service.Create(context.Background(), owner, &recordstest.Create{Name: name})
	require.NoError(t, err)

	return created.ID
}

type fakeReader struct {
	refs  []timeline.Ref
	total int
	err   error
}

func (f fakeReader) Page(
	context.Context, string, []kind.Kind, []string, string, string, int, string,
) (domain.Page[timeline.Ref], error) {
	if f.err != nil {
		return domain.Page[timeline.Ref]{}, f.err
	}

	return domain.NewPage(f.refs, nil), nil
}

func (f fakeReader) Count(context.Context, string, []kind.Kind, []string, string, string) (int, error) {
	return f.total, f.err
}

func TestListHydratesEveryRefInTheReadersOwnOrder(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)
	idOne := seed(t, registry, kindOne, "First")
	idTwo := seed(t, registry, kindTwo, "Second")

	reader := fakeReader{refs: []timeline.Ref{
		{Kind: kindTwo, RecordID: idTwo, OccurredOn: mustDate(t, "2026-01-10")},
		{Kind: kindOne, RecordID: idOne},
	}}

	svc, err := timeline.New(registry, reader)
	require.NoError(t, err)

	page, err := svc.List(context.Background(), owner, timeline.Query{PatientID: "any"})
	require.NoError(t, err)
	require.Len(t, page.Items, 2)

	assert.Equal(t, kindTwo, page.Items[0].Kind)
	assert.Equal(t, "Second", page.Items[0].Title)
	require.NotNil(t, page.Items[0].OccurredOn)
	assert.Equal(t, "2026-01-10", *page.Items[0].OccurredOn)

	assert.Equal(t, kindOne, page.Items[1].Kind)
	assert.Equal(t, "First", page.Items[1].Title)
	assert.Nil(t, page.Items[1].OccurredOn, "a ref with no primary date is occurred_on: null, never a substituted date")
}

func TestListSkipsARefWhoseRecordIsGone(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	reader := fakeReader{refs: []timeline.Ref{{Kind: kindOne, RecordID: "does-not-exist"}}}

	svc, err := timeline.New(registry, reader)
	require.NoError(t, err)

	page, err := svc.List(context.Background(), owner, timeline.Query{PatientID: "any"})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestListRefusesAStrangerExactlyAsNotFound(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	svc, err := timeline.New(registry, fakeReader{})
	require.NoError(t, err)

	stranger := access.Actor{UserID: "somebody-else"}

	_, err = svc.List(context.Background(), stranger, timeline.Query{PatientID: "any"})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListRejectsAnUnregisteredKindSelection(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	svc, err := timeline.New(registry, fakeReader{})
	require.NoError(t, err)

	_, err = svc.List(context.Background(), owner, timeline.Query{PatientID: "any", Kinds: []kind.Kind{"nope"}})
	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
}

func TestListAppliesTheCountWhenAsked(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	svc, err := timeline.New(registry, fakeReader{total: 7})
	require.NoError(t, err)

	page, err := svc.List(context.Background(), owner, timeline.Query{PatientID: "any", Count: true})
	require.NoError(t, err)
	require.NotNil(t, page.Total)
	assert.Equal(t, 7, *page.Total)
}

func TestNewRefusesAMissingCollaborator(t *testing.T) {
	t.Parallel()

	registry := newRegistry(t)

	_, err := timeline.New(nil, fakeReader{})
	assert.Error(t, err)

	_, err = timeline.New(registry, nil)
	assert.Error(t, err)
}

func mustDate(t *testing.T, s string) domain.Date {
	t.Helper()

	var d domain.Date
	require.NoError(t, d.UnmarshalText([]byte(s)))

	return d
}
