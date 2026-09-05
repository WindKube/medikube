package emergencycontact_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
	pbemergencycontact "medikube/internal/store/emergencycontact"

	_ "medikube/internal/store/migrations"
)

// T041. Deleting a patient destroys the emergency contacts filed against it,
// the same cascade patient's own test proves for medications.
func TestDeletingAPatientDestroysItsEmergencyContacts(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	owner := seedAccount(t, app, "contact-cascade-owner@example.test")
	patient := seedPatient(t, app, owner)

	secret, err := store.CursorSecret(app, "")
	require.NoError(t, err)

	codec, err := store.NewCursorCodec(secret)
	require.NoError(t, err)

	repo, err := pbemergencycontact.New(app, codec)
	require.NoError(t, err)

	_, err = repo.Create(t.Context(), clinical.EmergencyContact{
		PatientID: patient, Name: "Ngozi Okonkwo", Relationship: clinical.ContactRelationshipSpouse, Phone: "+1-555-0100",
	})
	require.NoError(t, err)

	patientRecord, err := app.FindRecordById(store.PatientCollection, patient)
	require.NoError(t, err)
	require.NoError(t, app.Delete(patientRecord))

	count, err := store.CountByPatient(t.Context(), app, kind.EmergencyContact.Collection(), patient)
	require.NoError(t, err)
	assert.Zero(t, count, "an emergency contact survived the patient it belonged to")
}

// T041's other half: emergency_contacts(patient) WHERE is_primary = 1 is a
// partial unique index, the storage-level backstop behind the service's own
// transactional displacement (research D-16). Writing two primary rows
// straight to the collection — bypassing that displacement — proves the
// backstop rather than the service logic sitting in front of it.
func TestAtMostOnePrimaryContactPerPatientIsEnforcedByTheIndex(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	owner := seedAccount(t, app, "contact-unique-owner@example.test")
	patient := seedPatient(t, app, owner)

	collection, err := app.FindCollectionByNameOrId(kind.EmergencyContact.Collection())
	require.NoError(t, err)

	first := core.NewRecord(collection)
	first.Set("patient", patient)
	first.Set("name", "Ngozi Okonkwo")
	first.Set("relationship", string(clinical.ContactRelationshipSpouse))
	first.Set("phone", "+1-555-0100")
	first.Set("is_primary", true)
	require.NoError(t, app.Save(first))

	second := core.NewRecord(collection)
	second.Set("patient", patient)
	second.Set("name", "Thandiwe Nakamura")
	second.Set("relationship", string(clinical.ContactRelationshipParent))
	second.Set("phone", "+1-555-0101")
	second.Set("is_primary", true)

	assert.Error(t, app.Save(second), "a second primary contact for the same patient was accepted")
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
