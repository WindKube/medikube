package allergy_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	pballergy "medikube/internal/store/allergy"

	_ "medikube/internal/store/migrations"
)

// T041. Deleting a patient destroys the allergies filed against it: the
// migration's CascadeDelete relation, proven the way patient's own cascade
// test proves it for medications.
func TestDeletingAPatientDestroysItsAllergies(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	owner := seedAccount(t, app, "allergy-cascade-owner@example.test")
	patient := seedPatient(t, app, owner)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pballergy.New(app, codec)
	require.NoError(t, err)

	_, err = repo.Create(t.Context(), clinical.Allergy{
		PatientID: patient, Allergen: "Penicillin", Severity: clinical.SeverityMild, Status: clinical.ConditionStatusActive,
	})
	require.NoError(t, err)

	patientRecord, err := app.FindRecordById(store.PatientCollection, patient)
	require.NoError(t, err)
	require.NoError(t, app.Delete(patientRecord))

	count, err := store.CountByPatient(t.Context(), app, kind.Allergy.Collection(), patient)
	require.NoError(t, err)
	assert.Zero(t, count, "an allergy survived the patient it belonged to")
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

func seedPatient(t *testing.T, app core.App, ownerID string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", "Test")
	record.Set("last_name", "Patient")

	require.NoError(t, app.Save(record))

	return record.Id
}
