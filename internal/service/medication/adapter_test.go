package medication_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/audit"
	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
	"medikube/internal/records/recordstest"
	"medikube/internal/service/medication"
	"medikube/internal/service/medication/medicationtest"
)

func wiring(t *testing.T, h harness) medication.Wiring {
	t.Helper()

	return medication.Wiring{
		Repository: h.repository,
		Authorizer: h.authorizer,
		Auditor:    h.auditor,
		Codec:      medicationtest.NewCodec(),
		Schema:     medicationtest.Shapes(),
		Views:      recordstest.Views{},
	}
}

func newAdapter(t *testing.T, h harness) *medication.Adapter {
	t.Helper()

	adapter, err := medication.NewAdapter(h.service, medicationtest.NewCodec())
	require.NoError(t, err)

	return adapter
}

// TestRegisterWiresEveryConsumer is the registry's own promise read from this
// side: one call registers the kind and everything that consumes it, and the
// spellings come from the kind table rather than from here.
func TestRegisterWiresEveryConsumer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	registry := records.NewRegistry()

	require.NoError(t, medication.Register(registry, wiring(t, h)))

	entry, registered := registry.FromKind(kind.Medication)
	require.True(t, registered)

	assert.Equal(t, kind.Medication.Segment(), entry.Segment)
	assert.Equal(t, kind.Medication.Collection(), entry.Collection)
	assert.False(t, entry.Synthetic)

	assert.NotNil(t, entry.Service, records.ConsumerRoutes)
	assert.NotNil(t, entry.Views, records.ConsumerViews)
	assert.NotNil(t, entry.Stream, records.ConsumerStream)
	assert.NotNil(t, entry.Authorizer, records.ConsumerAuthorizer)
	assert.Equal(t, audit.TargetKindMedication, entry.Target, records.ConsumerAudit)
	assert.NotEmpty(t, entry.Inventory.Title, records.ConsumerInventory)
	assert.NotEmpty(t, entry.Inventory.Summary, records.ConsumerInventory)

	require.NotNil(t, entry.Schema.NewSummary, records.ConsumerSchema)
	require.NotNil(t, entry.Schema.NewDetail, records.ConsumerSchema)
	require.NotNil(t, entry.Schema.NewCreate, records.ConsumerSchema)
	require.NotNil(t, entry.Schema.NewPatch, records.ConsumerSchema)

	// The vocabulary the service enforces and the vocabulary OpenAPI publishes
	// are one list, filled in by Register and not by the caller.
	assert.Equal(t, medication.Sorts(), entry.Schema.Sorts)
	assert.Equal(t, []string{medication.FilterStatus}, entry.Schema.Filters)
	assert.Equal(t, medication.Sorts()[0], entry.Schema.Sorts[0], "the published default ordering moved")
}

// TestRegisterRefusesAnIncompleteWiring, one absent dependency at a time. A
// registration that reached the router with no views is a page that panics on
// its first request, and it looks exactly like a working boot until then.
func TestRegisterRefusesAnIncompleteWiring(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		remove func(*medication.Wiring)
	}{
		{name: "no repository", remove: func(w *medication.Wiring) { w.Repository = nil }},
		{name: "no auditor", remove: func(w *medication.Wiring) { w.Auditor = nil }},
		{name: "no codec", remove: func(w *medication.Wiring) { w.Codec = nil }},
		{name: "no views", remove: func(w *medication.Wiring) { w.Views = nil }},
		{name: "no authorizer", remove: func(w *medication.Wiring) { w.Authorizer = nil }},
		{name: "no DTO shapes", remove: func(w *medication.Wiring) { w.Schema = records.Schema{} }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			incomplete := wiring(t, h)
			testCase.remove(&incomplete)

			registry := records.NewRegistry()

			require.Error(t, medication.Register(registry, incomplete))
			assert.Empty(t, registry.Kinds(), "a refused registration left the kind half registered")
		})
	}
}

func TestRegisterRefusesTheSameKindTwice(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	registry := records.NewRegistry()

	require.NoError(t, medication.Register(registry, wiring(t, h)))
	require.ErrorIs(t, medication.Register(registry, wiring(t, h)), records.ErrAlreadyRegistered)
}

// TestTheAdapterCarriesAPageThroughUnchanged. The envelope is the service's:
// the adapter replaces the entity with the kind's own DTO and touches nothing
// else, because a cursor rewritten on the way out is a cursor the next request
// cannot continue from.
func TestTheAdapterCarriesAPageThroughUnchanged(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	adapter := newAdapter(t, h)

	for _, name := range []string{"Amoxicillin", "Bisoprolol", "Codeine"} {
		h.store(t, name)
	}

	page, err := adapter.List(t.Context(), actor(), records.Query{Limit: 2, Count: true})
	require.NoError(t, err)

	require.Len(t, page.Items, 2)
	require.NotNil(t, page.NextCursor)
	require.NotNil(t, page.Total)
	assert.Equal(t, 3, *page.Total)

	for _, item := range page.Items {
		assert.Equal(t, kind.Medication, item.Kind)
		assert.NotEmpty(t, item.ID)
		assert.NotEmpty(t, item.Version, "no version on a listed record means no ETag on the row")

		summary, isSummary := item.Body.(*medicationtest.Summary)
		require.True(t, isSummary, "the list body is %T and not the kind's own summary", item.Body)
		assert.Equal(t, item.ID, summary.ID)
		assert.Equal(t, kind.Medication.Enum(), summary.Kind, "the discriminator is the enum spelling")
	}
}

// TestTheAdapterNarrowsByThePublishedParameter. The generic handler checks that
// the parameter is one the kind published; converting its values is the
// adapter's, and refusing them is the service's.
func TestTheAdapterNarrowsByThePublishedParameter(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	adapter := newAdapter(t, h)

	page, err := adapter.List(t.Context(), actor(), records.Query{
		Filters: map[string][]string{medication.FilterStatus: {string(clinical.TherapyStatusStopped)}},
	})
	require.NoError(t, err)
	assert.Empty(t, page.Items)

	assert.Equal(t,
		[]clinical.TherapyStatus{clinical.TherapyStatusStopped},
		h.repository.LastQuery().Statuses)

	_, err = adapter.List(t.Context(), actor(), records.Query{
		Filters: map[string][]string{medication.FilterStatus: {"lapsed"}},
	})

	var invalid *domain.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, medication.FilterStatus, invalid.Fields[0].Field)
}

// TestTheAdapterDecodesThroughTheCodec, in both directions and for both write
// operations.
func TestTheAdapterDecodesThroughTheCodec(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	adapter := newAdapter(t, h)

	created, err := adapter.Create(t.Context(), actor(), &medicationtest.Create{Name: "Amoxicillin", Dosage: "500 mg"})
	require.NoError(t, err)

	detail, isDetail := created.Body.(*medicationtest.Detail)
	require.True(t, isDetail, "the create body is %T and not the kind's own detail", created.Body)
	assert.Equal(t, "Amoxicillin", detail.Name)
	assert.Equal(t, "500 mg", detail.Dosage)
	assert.Equal(t, created.ID, detail.ID)

	dosage := "250 mg"

	updated, err := adapter.Update(t.Context(), actor(), created.ID, created.Version, &medicationtest.Patch{Dosage: &dosage})
	require.NoError(t, err)

	changed, isDetail := updated.Body.(*medicationtest.Detail)
	require.True(t, isDetail)
	assert.Equal(t, "250 mg", changed.Dosage)
	assert.Equal(t, "Amoxicillin", changed.Name, "an unsupplied field was changed on the way through")
	assert.NotEqual(t, created.Version, updated.Version, "the version did not move after a write")

	read, err := adapter.Get(t.Context(), actor(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, updated.Version, read.Version)

	require.NoError(t, adapter.Delete(t.Context(), actor(), created.ID, updated.Version))

	_, err = adapter.Get(t.Context(), actor(), created.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// TestABodyTheKindDidNotMintIsAWiringFailure. The generic handler decodes into
// the type the registration's own Schema minted, so a mismatch here is the
// registration being wrong and not the request.
func TestABodyTheKindDidNotMintIsAWiringFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	adapter := newAdapter(t, h)

	_, err := adapter.Create(t.Context(), actor(), &recordstest.Create{Name: "Amoxicillin"})
	require.Error(t, err)
	assert.Empty(t, h.repository.Writes())

	_, err = adapter.Update(t.Context(), actor(), "somerecord0001", "some-version", &recordstest.Patch{})
	require.Error(t, err)
	assert.Empty(t, h.repository.Writes())
}

// TestTheStreamFilterAdmitsAChangeAndRefusesAnEventThatNamesNothing.
func TestTheStreamFilterAdmitsAChangeAndRefusesAnEventThatNamesNothing(t *testing.T) {
	t.Parallel()

	filter := medication.StreamFilter{}

	assert.True(t, filter.Streams("somerecord0001", medicationtest.OwnerID))
	assert.False(t, filter.Streams("", medicationtest.OwnerID), "an event naming no record was admitted")
	assert.False(t, filter.Streams("somerecord0001", ""), "an event naming no owner was admitted")
}
