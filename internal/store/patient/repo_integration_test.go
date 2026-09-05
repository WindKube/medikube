package patient_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/person"
	"medikube/internal/service/patient"
	"medikube/internal/service/patient/patienttest"
	"medikube/internal/store"
	pbpatient "medikube/internal/store/patient"

	// See internal/store/medication/repo_integration_test.go's own comment:
	// the migrations register themselves from their own init.
	_ "medikube/internal/store/migrations"
)

const (
	ownerEmail    = "patient-owner@example.test"
	strangerEmail = "patient-stranger@example.test"
)

func TestRepositorySatisfiesTheContract(t *testing.T) {
	t.Parallel()

	patienttest.RepositoryContract(t, func(t *testing.T) patient.Repository {
		t.Helper()

		app, err := tests.NewTestApp(t.TempDir())
		require.NoError(t, err)
		t.Cleanup(app.Cleanup)

		secret, err := store.CursorSecret(app, "")
		require.NoError(t, err)

		codec, err := store.NewCursorCodec(secret)
		require.NoError(t, err)

		repo, err := pbpatient.New(app, codec)
		require.NoError(t, err)

		remapAccounts(t, app)

		return repo
	})
}

// remapAccounts writes patienttest.OwnerID and patienttest.StrangerID as real
// account records, so the contract's fixed owner ids satisfy the `owner`
// relation's existence check against a real database — the fake has no such
// check at all.
func remapAccounts(t *testing.T, app core.App) {
	t.Helper()

	seedAccountWithID(t, app, patienttest.OwnerID, ownerEmail)
	seedAccountWithID(t, app, patienttest.StrangerID, strangerEmail)
}

func seedAccountWithID(t *testing.T, app core.App, id, email string) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	require.NoError(t, err)

	record := core.NewRecord(users)
	record.Id = id
	record.SetEmail(email)
	record.SetPassword("correct-horse-battery-staple")
	record.Set("name", "Test Person")
	record.Set("role", "user")
	record.Set("unit_system", "metric")
	record.Set("locale", "en")
	record.Set("date_format", "iso")
	record.Set("theme", "system")

	require.NoError(t, app.Save(record))
}

func TestPatientOwnerAnswersNotFoundForAMissingID(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbpatient.New(app, codec)
	require.NoError(t, err)

	_, err = repo.PatientOwner(t.Context(), "mkdoesnotexist1")
	require.Error(t, err)
}

func TestPatientOwnerAnswersTheOwningAccount(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	seedAccountWithID(t, app, patienttest.OwnerID, ownerEmail)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbpatient.New(app, codec)
	require.NoError(t, err)

	created, err := repo.Create(t.Context(), person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo",
	})
	require.NoError(t, err)

	owner, err := repo.PatientOwner(t.Context(), created.ID)
	require.NoError(t, err)
	require.Equal(t, patienttest.OwnerID, owner)
}
