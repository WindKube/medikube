package patienttest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
)

// RepositoryFactory builds a fresh, empty repository. internal/store/patient's
// integration test hands in one that clones tests.NewTestApp; this package's
// own test hands in NewRepository.
type RepositoryFactory func(t *testing.T) patient.Repository

// RepositoryContract is the shared Liskov suite every Repository
// implementation must pass, mirroring
// internal/service/medication/medicationtest.RunRepositoryContract.
func RepositoryContract(t *testing.T, factory RepositoryFactory) {
	t.Helper()

	t.Run("a created patient is owned and reachable", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		draft := person.Patient{OwnerID: OwnerID, FirstName: "Amara", LastName: "Okonkwo",
			BirthDate: mustDate(t, "1988-04-12")}

		created, err := repo.Create(ctx, draft)
		require.NoError(t, err)
		require.NotEmpty(t, created.ID)
		assert.Equal(t, OwnerID, created.OwnerID)
		assert.NotEmpty(t, created.Version)

		found, err := repo.Get(ctx, OwnerID, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, found.ID)
		assert.Equal(t, "Amara", found.FirstName)
	})

	t.Run("a stranger's Get is domain.ErrNotFound", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		created, err := repo.Create(ctx, person.Patient{
			OwnerID: OwnerID, FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12"),
		})
		require.NoError(t, err)

		_, err = repo.Get(ctx, StrangerID, created.ID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("an id that never existed is domain.ErrNotFound", func(t *testing.T) {
		repo := factory(t)

		_, err := repo.Get(t.Context(), OwnerID, "mkdoesnotexist1")
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("List answers only the owner's rows", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		mine, err := repo.Create(ctx, person.Patient{OwnerID: OwnerID, FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12")})
		require.NoError(t, err)

		_, err = repo.Create(ctx, person.Patient{OwnerID: StrangerID, FirstName: "Boris", LastName: "Novak", BirthDate: mustDate(t, "1990-07-22")})
		require.NoError(t, err)

		page, err := repo.List(ctx, OwnerID, patient.Query{})
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		assert.Equal(t, mine.ID, page.Items[0].ID)
	})

	t.Run("List paginates without skipping or duplicating a row", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		names := []string{"Ada", "Bo", "Chi", "Dara", "Eze"}
		created := make(map[string]bool, len(names))

		for _, name := range names {
			row, err := repo.Create(ctx, person.Patient{OwnerID: OwnerID, FirstName: name, LastName: "Test", BirthDate: mustDate(t, "1990-01-01")})
			require.NoError(t, err)
			created[row.ID] = false
		}

		var cursor string

		for range len(names) {
			page, err := repo.List(ctx, OwnerID, patient.Query{Limit: 1, Cursor: cursor})
			require.NoError(t, err)
			require.Len(t, page.Items, 1)

			id := page.Items[0].ID
			require.Falsef(t, created[id], "row %s was returned twice", id)
			created[id] = true

			if page.NextCursor == nil {
				break
			}

			cursor = *page.NextCursor
		}

		for id, seen := range created {
			assert.Truef(t, seen, "row %s was never returned", id)
		}
	})

	t.Run("Update refuses a stale version", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		created, err := repo.Create(ctx, person.Patient{OwnerID: OwnerID, FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12")})
		require.NoError(t, err)

		changed := created
		changed.LastName = "Adeyemi"

		_, err = repo.Update(ctx, changed, "not-the-real-version")
		assert.ErrorIs(t, err, domain.ErrVersionMismatch)
	})

	t.Run("Update applies over the current version and advances it", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		created, err := repo.Create(ctx, person.Patient{OwnerID: OwnerID, FirstName: "Amara", LastName: "Okonkwo", BirthDate: mustDate(t, "1988-04-12")})
		require.NoError(t, err)

		changed := created
		changed.LastName = "Adeyemi"

		// The version is derived from the record's updated timestamp
		// (internal/store.Version), millisecond-resolution: without this gap
		// a fast repository could save both writes in the same millisecond
		// and mint an identical version.
		time.Sleep(2 * time.Millisecond)

		updated, err := repo.Update(ctx, changed, created.Version)
		require.NoError(t, err)
		assert.Equal(t, "Adeyemi", updated.LastName)
		assert.NotEqual(t, created.Version, updated.Version)
	})

	t.Run("SelfRecord answers domain.ErrNotFound before one is created", func(t *testing.T) {
		repo := factory(t)

		_, err := repo.SelfRecord(t.Context(), OwnerID)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("SelfRecord finds the owner's is_self_record row", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		created, err := repo.Create(ctx, person.Patient{
			OwnerID: OwnerID, RelationshipToOwner: person.RelationshipSelf, IsSelfRecord: true,
		})
		require.NoError(t, err)

		found, err := repo.SelfRecord(ctx, OwnerID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, found.ID)
	})

	t.Run("Create refuses a second self-record for one owner", func(t *testing.T) {
		repo := factory(t)
		ctx := t.Context()

		_, err := repo.Create(ctx, person.Patient{OwnerID: OwnerID, RelationshipToOwner: person.RelationshipSelf, IsSelfRecord: true})
		require.NoError(t, err)

		_, err = repo.Create(ctx, person.Patient{OwnerID: OwnerID, RelationshipToOwner: person.RelationshipSelf, IsSelfRecord: true})
		assert.ErrorIs(t, err, domain.ErrConflict)
	})
}

func mustDate(t *testing.T, text string) domain.Date {
	t.Helper()

	date, err := domain.ParseDate(text)
	require.NoError(t, err)

	return date
}
