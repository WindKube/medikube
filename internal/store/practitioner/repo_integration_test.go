package practitioner_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/service/practitioner"
	"medikube/internal/service/practitioner/practitionertest"
	"medikube/internal/store"
	pbpractitioner "medikube/internal/store/practitioner"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance. Without
	// this import the collection this package reads does not exist.
	_ "medikube/internal/store/migrations"
)

const (
	ownerEmail    = "owner@example.test"
	strangerEmail = "stranger@example.test"
)

// harness is one instance, one repository and two accounts, mirroring
// internal/store/medication's own.
type harness struct {
	app      *tests.TestApp
	repo     *pbpractitioner.Repo
	owner    string
	stranger string
}

func newHarness(t *testing.T) harness {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbpractitioner.New(app, codec)
	require.NoError(t, err)

	return harness{
		app:      app,
		repo:     repo,
		owner:    seedAccount(t, app, ownerEmail),
		stranger: seedAccount(t, app, strangerEmail),
	}
}

func seedAccount(t *testing.T, app core.App, email string) string {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Test Person")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(t, app.Save(record))

	return record.Id
}

// seedFacility writes a minimal facility record owned by ownerID, so a
// facility reference has something real to point at (a relation with no
// cascade still has to name an existing row).
func seedFacility(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	facilities, err := app.FindCollectionByNameOrId("facilities")
	require.NoError(t, err)

	record := core.NewRecord(facilities)
	record.Set("owner", ownerID)
	record.Set("kind", "practice")
	record.Set("name", "Test Facility")

	require.NoError(t, app.Save(record))

	return record.Id
}

// T123. The PocketBase repository passes the same suite the in-memory fake
// passes.
func TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	practitionertest.RunRepositoryContract(t, func(t *testing.T) (practitioner.Repository, practitionertest.Accounts) {
		t.Helper()

		h := newHarness(t)

		return h.repo, practitionertest.Accounts{
			Owner:    h.owner,
			Stranger: h.stranger,
			SeedFacility: func(t *testing.T, ownerID string) string {
				t.Helper()

				return seedFacility(t, h.app, ownerID)
			},
		}
	})
}
