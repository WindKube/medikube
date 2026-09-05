package recordstest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/access"
	"medikube/internal/records"
)

// RepositoryHarness is one fresh instance of a kind's storage, per test: a
// factory and not a shared instance, so no case leaks a row into the next
// (the same rationale internal/service/medication/medicationtest/contract.go
// gives for its own per-test factory).
//
// It is built at the records.Service level and not against a kind's own
// Repository interface, because that is the one shape every kind actually
// shares — each kind's own Query and entity types differ, but every kind's
// Register wires exactly this. A lower-level, per-kind repository contract
// has no shared shape across kinds to write once.
type RepositoryHarness struct {
	Service records.Service

	// Owner is authorized for PatientID; Stranger is authorized for nobody
	// this harness writes, the same way a real kind's Authorizer refuses
	// another account (FR-033).
	Owner     access.Actor
	PatientID string
	Stranger  access.Actor
}

// RepositoryContractOptions parameterises RunRepositoryContract. Two clauses
// are optional, each gated by its own skip reason rather than an unexplained
// absence: a kind that cannot yet satisfy one documents why, on the suite,
// instead of the suite silently asserting less for it.
type RepositoryContractOptions struct {
	NewHarness func(t *testing.T) RepositoryHarness
	Fixture    Fixture

	// NewPatch mints an empty patch of the kind's own type — entry.Schema.
	// NewPatch, unevaluated — for the clauses that update a record without
	// caring what changes: every kind's patch fields are optional, so the
	// zero value is always a valid, if inert, update.
	NewPatch func() any

	// Sort is the ordering RunRepositoryContract pages under. It should be the
	// kind's own default: the identity tiebreaker (research D-06) is what
	// makes several Fixture.Minimal-built records — identical but for their
	// minted id — order deterministically at all.
	Sort []domain.SortKey

	// HasPrimaryDate reads Record.Body — the same DTO Views renders — and
	// reports whether the field the default sort orders by is present. It is
	// required unless NullPrimaryDateSkip documents why the clause does not
	// apply (e.g. the kind's primary date is required, so Fixture.Minimal
	// cannot omit it).
	HasPrimaryDate      func(body any) bool
	NullPrimaryDateSkip string

	// DeletePatient removes the patient every fixture record in this run was
	// filed against. It is required unless CascadeSkip documents why this
	// tier cannot exercise the cascade (e.g. deleting a patient is not a
	// records.Service operation, and this kind's cascade is proven instead by
	// an internal/store integration test against a real instance).
	DeletePatient func(ctx context.Context, patientID string) error
	CascadeSkip   string
}

// RunRepositoryContract is the storage-tier contract every registered kind's
// Service passes: not-found, a foreign patient's refusal, the version check,
// cursor stability under a concurrent insert, and where an entity's primary
// date is optional, that an unset one sorts last (research D-06).
func RunRepositoryContract(t *testing.T, opts RepositoryContractOptions) {
	t.Helper()

	require.NotNil(t, opts.NewHarness, "the contract has no harness factory to run against")
	require.NotNil(t, opts.Fixture.Minimal, "the contract has no minimal fixture to create with")
	require.NotNil(t, opts.NewPatch, "the contract has no patch constructor to update with")

	t.Run("get answers not found for an id that never existed", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		_, err := h.Service.Get(t.Context(), h.Owner, "no-such-record-id")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("get answers not found for another account's record", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		_, err = h.Service.Get(t.Context(), h.Stranger, created.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("update refuses a stale version", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		_, err = h.Service.Update(t.Context(), h.Owner, created.ID, "not-the-current-version", opts.NewPatch())
		assert.ErrorIs(t, err, domain.ErrVersionMismatch)
	})

	t.Run("delete refuses a stale version", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		err = h.Service.Delete(t.Context(), h.Owner, created.ID, "not-the-current-version")
		assert.ErrorIs(t, err, domain.ErrVersionMismatch)
	})

	t.Run("delete removes the record", func(t *testing.T) {
		t.Parallel()

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		require.NoError(t, h.Service.Delete(t.Context(), h.Owner, created.ID, created.Version))

		_, err = h.Service.Get(t.Context(), h.Owner, created.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("cursor pagination neither repeats nor skips under a concurrent insert", func(t *testing.T) {
		t.Parallel()

		require.NotEmpty(t, opts.Sort, "the contract has no sort to page under")

		h := opts.NewHarness(t)

		const seeded = 3

		preInsert := make(map[string]bool, seeded)
		for range seeded {
			record, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
			require.NoError(t, err)
			preInsert[record.ID] = true
		}

		first, err := h.Service.List(t.Context(), h.Owner, records.Query{
			PatientID: h.PatientID, Sort: opts.Sort, Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, first.Items, 1)
		require.NotNil(t, first.NextCursor, "one page of three at limit 1 has a next page")

		seen := map[string]bool{first.Items[0].ID: true}

		// The keyset tiebreaker is the minted id, which is not ordered by
		// insertion time — so this insert may land anywhere in the ordering
		// and is not asserted to appear. What must hold regardless is that
		// none of the three rows already sitting there when the walk began is
		// skipped or repeated by it.
		_, err = h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		cursor := *first.NextCursor
		for {
			page, err := h.Service.List(t.Context(), h.Owner, records.Query{
				PatientID: h.PatientID, Sort: opts.Sort, Limit: 1, Cursor: cursor,
			})
			require.NoError(t, err)

			for _, item := range page.Items {
				assert.Falsef(t, seen[item.ID], "%s was already returned by an earlier page", item.ID)
				seen[item.ID] = true
			}

			if page.NextCursor == nil {
				break
			}

			cursor = *page.NextCursor
		}

		for id := range preInsert {
			assert.Truef(t, seen[id], "%s existed before the walk began and was not returned by it", id)
		}
	})

	t.Run("an undated record sorts last", func(t *testing.T) {
		t.Parallel()

		if opts.NullPrimaryDateSkip != "" {
			t.Skip(opts.NullPrimaryDateSkip)
		}

		require.NotNil(t, opts.HasPrimaryDate, "NullPrimaryDateSkip is empty but HasPrimaryDate is nil")
		require.NotEmpty(t, opts.Sort, "the contract has no sort to order by")

		h := opts.NewHarness(t)

		undated, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)
		assert.False(t, opts.HasPrimaryDate(undated.Body),
			"Fixture.Minimal set the field the default sort orders by, so this clause proves nothing")

		dated, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Full(h.PatientID))
		require.NoError(t, err)
		assert.True(t, opts.HasPrimaryDate(dated.Body))

		page, err := h.Service.List(t.Context(), h.Owner, records.Query{PatientID: h.PatientID, Sort: opts.Sort, Limit: 10})
		require.NoError(t, err)
		require.Len(t, page.Items, 2)

		assert.Equal(t, undated.ID, page.Items[len(page.Items)-1].ID,
			"the undated record did not sort last")
	})

	t.Run("records vanish when the patient is deleted", func(t *testing.T) {
		t.Parallel()

		if opts.CascadeSkip != "" {
			t.Skip(opts.CascadeSkip)
		}

		require.NotNil(t, opts.DeletePatient, "CascadeSkip is empty but DeletePatient is nil")

		h := opts.NewHarness(t)

		created, err := h.Service.Create(t.Context(), h.Owner, opts.Fixture.Minimal(h.PatientID))
		require.NoError(t, err)

		require.NoError(t, opts.DeletePatient(t.Context(), h.PatientID))

		_, err = h.Service.Get(t.Context(), h.Owner, created.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}
