package records_test

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
	"medikube/internal/service/search"
)

// fakeSearchReader is search.Reader over a fixed slice of refs, for a test
// that wants to drive Handler.List's merge and hydrate without a database.
type fakeSearchReader struct {
	refs  []search.Ref
	next  *string
	err   error
	total int
}

func (f fakeSearchReader) Page(context.Context, string, []kind.Kind, int, string) (domain.Page[search.Ref], error) {
	if f.err != nil {
		return domain.Page[search.Ref]{}, f.err
	}

	return domain.NewPage(f.refs, f.next), nil
}

func (f fakeSearchReader) Count(context.Context, string, []kind.Kind) (int, error) {
	return f.total, nil
}

// crossKindHarness registers medication and the synthetic fake_kind, both
// against fakes this test keeps its own handle to, so it can create records
// through them directly and mint refs the fake reader answers with.
type crossKindHarness struct {
	registry *records.Registry
	medicine *recordstest.FakeKindService
	fake     *recordstest.FakeKindService
}

func newCrossKindHarness(t *testing.T) crossKindHarness {
	t.Helper()

	registry := records.NewRegistry()

	medReg := recordstest.Registration(kind.Medication, audit.TargetKindMedication)
	require.NoError(t, registry.Register(medReg))

	fakeReg := recordstest.SyntheticRegistration()
	require.NoError(t, registry.RegisterSynthetic(fakeReg, recordstest.Segment, recordstest.Collection))

	medicine, ok := medReg.Service.(*recordstest.FakeKindService)
	require.True(t, ok, "recordstest.Registration wires something other than a *FakeKindService")

	fake, ok := fakeReg.Service.(*recordstest.FakeKindService)
	require.True(t, ok, "recordstest.SyntheticRegistration wires something other than a *FakeKindService")

	return crossKindHarness{registry: registry, medicine: medicine, fake: fake}
}

// T-crosskind. The cross-kind list merges refs from more than one kind's
// index rows and hydrates each through that kind's own Service.Get — which is
// where FR-033's ownership check already lives — skipping a ref whose record
// is gone rather than failing the whole page.
func TestTheCrossKindListMergesAndHydratesTwoKinds(t *testing.T) {
	t.Parallel()

	h := newCrossKindHarness(t)

	medRecord, err := h.medicine.Create(t.Context(), owner(), &recordstest.Create{Name: "med-1"})
	require.NoError(t, err)

	fakeRecord, err := h.fake.Create(t.Context(), owner(), &recordstest.Create{Name: "fake-1"})
	require.NoError(t, err)

	h.registry.SetSearchReader(fakeSearchReader{refs: []search.Ref{
		{Kind: kind.Medication, RecordID: medRecord.ID},
		// Deleted between the index read and this hydrate: skipped, not an
		// error.
		{Kind: recordstest.Kind, RecordID: "mkfakerecordgone"},
		{Kind: recordstest.Kind, RecordID: fakeRecord.ID},
	}})

	handler := records.NewHandler(h.registry)

	page, err := handler.List(context.Background(), owner(), records.Query{
		Kinds:     []kind.Kind{kind.Medication, recordstest.Kind},
		PatientID: "patient-1",
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}

	assert.ElementsMatch(t, []string{medRecord.ID, fakeRecord.ID}, ids)
}

// The cross-kind list publishes one ordering. A `sort` other than it is 422
// invalid_value, exactly as an unpublished sort is on a single kind's list.
func TestTheCrossKindListRefusesASortOtherThanTheDefault(t *testing.T) {
	t.Parallel()

	h := newCrossKindHarness(t)
	h.registry.SetSearchReader(fakeSearchReader{})

	handler := records.NewHandler(h.registry)

	_, err := handler.List(context.Background(), owner(), records.Query{
		Kinds:     []kind.Kind{kind.Medication, recordstest.Kind},
		PatientID: "patient-1",
		Sort:      []domain.SortKey{{Field: "name"}},
	})

	require.Error(t, err)

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	require.Len(t, invalid.Fields, 1)
	assert.Equal(t, "sort", invalid.Fields[0].Field)
	assert.Equal(t, domain.CodeInvalidValue, invalid.Fields[0].Code)
}

// The cross-kind list has no search and no named filters of its own: both are
// refused with domain.ErrBadRequest, the same code path an unpublished filter
// on a single kind's list takes.
func TestTheCrossKindListRefusesSearchAndFilters(t *testing.T) {
	t.Parallel()

	h := newCrossKindHarness(t)
	h.registry.SetSearchReader(fakeSearchReader{})

	handler := records.NewHandler(h.registry)

	t.Run("q", func(t *testing.T) {
		t.Parallel()

		_, err := handler.List(context.Background(), owner(), records.Query{
			Kinds: []kind.Kind{kind.Medication, recordstest.Kind}, PatientID: "patient-1", Search: "aspirin",
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrBadRequest)
	})

	t.Run("named filter", func(t *testing.T) {
		t.Parallel()

		_, err := handler.List(context.Background(), owner(), records.Query{
			Kinds: []kind.Kind{kind.Medication, recordstest.Kind}, PatientID: "patient-1",
			Filters: map[string][]string{"status": {"active"}},
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrBadRequest)
	})
}

// The patient scope is authorized once, up front, against the selection —
// before the index is ever paged — so a stranger naming somebody else's
// patient is refused the list outright and not handed an empty one.
func TestTheCrossKindListAuthorizesThePatientBeforePagingTheIndex(t *testing.T) {
	t.Parallel()

	h := newCrossKindHarness(t)
	h.registry.SetSearchReader(fakeSearchReader{refs: []search.Ref{{Kind: kind.Medication, RecordID: "whatever"}}})

	handler := records.NewHandler(h.registry)

	stranger := access.Actor{UserID: "someone-else", RequestID: "req-2"}

	_, err := handler.List(context.Background(), stranger, records.Query{
		Kinds: []kind.Kind{kind.Medication, recordstest.Kind}, PatientID: "patient-1",
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
