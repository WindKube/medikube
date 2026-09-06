package tag_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	dtag "medikube/internal/domain/tag"
	"medikube/internal/service/tag"
	"medikube/internal/service/tag/tagtest"
	"medikube/internal/store"
	pbtag "medikube/internal/store/tag"

	// The migrations register themselves from their own init and
	// tests.NewTestApp runs core.AppMigrations against the instance.
	_ "medikube/internal/store/migrations"
)

const (
	ownerEmail    = "tagowner@example.test"
	strangerEmail = "tagstranger@example.test"
)

type harness struct {
	app      *tests.TestApp
	repo     *pbtag.Repo
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

	repo, err := pbtag.New(app, codec, func() []string { return nil })
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

// T153. The PocketBase repository passes the same suite the in-memory fake
// passes, including the unique index on (owner, LOWER(name)) enforced at the
// storage layer.
func TestThePocketBaseRepositoryPassesTheSameContractTheFakeDoes(t *testing.T) {
	t.Parallel()

	tagtest.RunRepositoryContract(t, func(t *testing.T) (tag.Repository, tagtest.Accounts) {
		t.Helper()

		h := newHarness(t)

		return h.repo, tagtest.Accounts{Owner: h.owner, Stranger: h.stranger}
	})
}

// T153: deleting a tag removes it from every referencing record while
// destroying none (FR-066, SC-007).
func TestDeletingATagRemovesItFromEveryReferencingRecordAndDestroysNone(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	ctx := t.Context()

	created, err := h.repo.Create(ctx, dtag.Tag{OwnerID: h.owner, Name: "cardiology"})
	require.NoError(t, err)

	patients, err := h.app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	patient := core.NewRecord(patients)
	patient.Set("owner", h.owner)
	patient.Set("first_name", "Amara")
	patient.Set("last_name", "Test")
	patient.Set("sex", "female")
	patient.Set("date_of_birth", "1990-01-01")
	require.NoError(t, h.app.Save(patient))

	medications, err := h.app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	medication := core.NewRecord(medications)
	medication.Set("patient", patient.Id)
	medication.Set("name", "Aspirin")
	medication.Set("type", "otc")
	medication.Set("status", "active")
	medication.Set("tags", []string{created.ID})
	require.NoError(t, h.app.Save(medication))

	require.NoError(t, h.repo.Delete(ctx, h.owner, created.ID))

	reread, err := h.app.FindRecordById(kind.Medication.Collection(), medication.Id)
	require.NoError(t, err, "the medication itself must survive the tag's deletion")
	assert.Empty(t, reread.GetStringSlice("tags"))
}
