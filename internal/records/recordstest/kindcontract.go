package recordstest

import (
	json "encoding/json/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
	"medikube/internal/records"
)

// KindContractOptions parameterises RunKindContract. Every clause runs; one
// that this tier cannot yet prove for a given kind is a documented skip
// (a non-empty *Skip reason, asserted via t.Skip so it is visible in the
// suite's own output) rather than an absent case.
type KindContractOptions struct {
	NewHarness func(t *testing.T) RepositoryHarness
	Entry      records.Entry
	Fixture    Fixture

	// DefaultSort is what an absent `sort` resolves to: entry.Schema.Sorts[0]
	// alone for a kind whose ordering is several single-field alternatives, or
	// the whole of entry.Schema.DefaultSort for the kind whose ordering is one
	// fixed compound (FR-051).
	DefaultSort []domain.SortKey

	// NoPatient builds a create body naming no patient, to prove FR-002's
	// refusal. Required unless NoPatientSkip documents why a kind cannot
	// build one generically (there is none: every kind is patient-scoped).
	NoPatient     func() any
	NoPatientSkip string

	// SearchIndex reads back the search_index row for one record, when this
	// harness wired a real Indexer. Nil and SearchIndexSkip non-empty
	// documents why this kind cannot yet prove it — T030's wiring is generic,
	// but proving it needs a real search.Repository, which is an
	// internal/store concern most kinds' unit-level harness does not carry.
	SearchIndex     func(t *testing.T, k kind.Kind, recordID string) (found bool, title string)
	SearchIndexSkip string
}

// RunKindContract is the registration-tier contract every registered kind
// passes: the registry accepted a complete registration for it, the six
// generic operations serve it (FR-001), a record cannot be filed against
// nobody (FR-002), its declared default sort is what the registry publishes,
// and — where wired — its writes and deletes keep search_index in step
// (research D-11).
//
// An audited row and a published realtime event are deliberately not asserted
// here: both are bound generically over every registered kind's collection by
// internal/platform/pb (hooks_records.go, BindRecordStream), against a real
// PocketBase instance, which this suite's harness does not require. They are
// proven once, for every kind at once, by internal/platform/pb's own tests
// and by contracts/records-clinical.md's HTTP-tier suites — asserting them
// again here per kind would not catch a fault either of those misses.
func RunKindContract(t *testing.T, opts KindContractOptions) {
	t.Helper()

	require.NotNil(t, opts.NewHarness, "the contract has no harness factory to run against")
	require.NotNil(t, opts.Fixture.Minimal, "the contract has no minimal fixture to create with")

	t.Run("the registration is complete", func(t *testing.T) {
		t.Parallel()

		AssertRegistrationComplete(t, opts.Entry)
	})

	t.Run("the six generic operations serve the kind", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		// Not asserted empty: NewHarness may share its patient with another
		// of this suite's subtests running in parallel (a real instance has
		// no cheap way to mint a fresh patient per case), so what listRecords
		// OfKind owes here is that the record this case creates is IN the
		// list, not that the list contains nothing else.
		_, err := h.Service.List(t.Context(), h.Owner, records.Query{PatientID: h.PatientID, Sort: opts.DefaultSort, Limit: 100})
		require.NoError(t, err, "listRecordsOfKind")

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err, "createRecord")
		assert.Equal(t, opts.Entry.Kind, created.Kind)

		listed, err := h.Service.List(t.Context(), h.Owner, records.Query{PatientID: h.PatientID, Sort: opts.DefaultSort, Limit: 100})
		require.NoError(t, err, "listRecordsOfKind")
		assert.Truef(t, containsID(listed.Items, created.ID), "the created record was not in its own kind's list")

		_, err = h.Service.Get(t.Context(), h.Owner, created.ID)
		require.NoError(t, err, "getRecord")

		updated, err := h.Service.Update(t.Context(), h.Owner, created.ID, created.Version, opts.Entry.Schema.NewPatch())
		require.NoError(t, err, "updateRecord")

		require.NoError(t, h.Service.Delete(t.Context(), h.Owner, created.ID, updated.Version), "deleteRecord")

		_, err = h.Service.Get(t.Context(), h.Owner, created.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("a record cannot be filed against nobody", func(t *testing.T) {
		t.Parallel()

		if opts.NoPatientSkip != "" {
			t.Skip(opts.NoPatientSkip)
		}

		require.NotNil(t, opts.NoPatient, "NoPatientSkip is empty but NoPatient is nil")

		h := opts.NewHarness(t)

		_, err := h.Service.Create(t.Context(), h.Owner, opts.NoPatient())

		var invalid *domain.ValidationError
		assert.ErrorAs(t, err, &invalid)
	})

	t.Run("a patch cannot re-attribute a record", func(t *testing.T) {
		t.Parallel()

		patch := opts.Entry.Schema.NewPatch()
		err := json.Unmarshal([]byte(`{"patient":"someone-else"}`), patch, json.RejectUnknownMembers(true))
		assert.Error(t, err, "the patch DTO accepted a patient member, so a request could re-file a record against a different person")
	})

	t.Run("the declared default sort is the one the registry publishes", func(t *testing.T) {
		t.Parallel()

		require.NotEmpty(t, opts.Entry.Schema.Sorts)

		expected := opts.Entry.Schema.DefaultSort
		if len(expected) == 0 {
			expected = []domain.SortKey{opts.Entry.Schema.Sorts[0]}
		}

		assert.Equal(t, expected, opts.DefaultSort)
	})

	t.Run("the owner reaches the record and a stranger cannot", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		_, err = h.Service.Get(t.Context(), h.Owner, created.ID)
		assert.NoError(t, err)

		_, err = h.Service.Get(t.Context(), h.Stranger, created.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("a create and a delete keep the search index in step", func(t *testing.T) {
		t.Parallel()

		if opts.SearchIndexSkip != "" {
			t.Skip(opts.SearchIndexSkip)
		}

		require.NotNil(t, opts.SearchIndex, "SearchIndexSkip is empty but SearchIndex is nil")

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Full(h.PatientID))
		require.NoError(t, err)

		found, title := opts.SearchIndex(t, opts.Entry.Kind, created.ID)
		assert.True(t, found, "no search_index row was written on create")
		assert.NotEmpty(t, title)

		require.NoError(t, h.Service.Delete(t.Context(), h.Owner, created.ID, created.Version))

		found, _ = opts.SearchIndex(t, opts.Entry.Kind, created.ID)
		assert.False(t, found, "the search_index row survived the record's delete")
	})
}

// AssertRegistrationComplete is what T021's build-wide sweep
// (registry_completeness_test.go) and RunKindContract both assert a
// registration owes, so the two read from one definition of "complete"
// rather than two that could drift apart.
func AssertRegistrationComplete(t *testing.T, entry records.Entry) {
	t.Helper()

	assert.NotEmpty(t, entry.Segment, "no path segment")
	assert.NotEmpty(t, entry.Collection, "no collection")
	assert.NotNil(t, entry.Service, "no service")

	assert.NotNil(t, entry.Schema.NewSummary, "no summary constructor")
	assert.NotNil(t, entry.Schema.NewDetail, "no detail constructor")
	assert.NotNil(t, entry.Schema.NewCreate, "no create constructor")
	assert.NotNil(t, entry.Schema.NewPatch, "no patch constructor")
	assert.NotEmpty(t, entry.Schema.Sorts, "no default sort")

	assert.NotNil(t, entry.Views, "no page views")
	assert.NotNil(t, entry.Stream, "no stream filter")
	assert.True(t, entry.Target.Valid(), "no valid audit target")
	assert.NotNil(t, entry.Authorizer, "no authorizer")
	assert.NotEmpty(t, entry.Inventory.Title, "no CLI inventory title")
	assert.NotEmpty(t, entry.Inventory.Summary, "no CLI inventory summary")

	assert.NotNil(t, entry.SearchFields, "no search fields")
	assert.NotNil(t, entry.Basis, "no basis")
	assert.NotEmpty(t, entry.SeedFixtureID, "no seed fixture id")
}

func containsID(items []records.Record, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}

	return false
}
