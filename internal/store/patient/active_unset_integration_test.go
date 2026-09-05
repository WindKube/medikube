package patient_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/person"
	"medikube/internal/service/patient/patienttest"
	"medikube/internal/store"

	_ "medikube/internal/store/migrations"
)

// T094/research D-07. users.active_patient's CascadeDelete is false
// (1756200400_users_active_patient.go): deleting a patient a person pointed
// at leaves the account itself alone and only clears the pointer, on every
// account that pointed at it — this is PocketBase's own relation-field
// behavior, asserted here directly against the schema rather than through a
// service that does not exist yet (US6's deletePatient).
func TestDeletingAPatientNullsTheActivePatientPointerEverywhereItPointed(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	remapAccounts(t, app)

	patients, err := app.FindCollectionByNameOrId(store.PatientCollection)
	require.NoError(t, err)

	target := core.NewRecord(patients)
	require.NoError(t, store.PatientToRecord(target, person.Patient{
		OwnerID: patienttest.OwnerID, FirstName: "Amara", LastName: "Okonkwo", IsSelfRecord: true,
	}))
	require.NoError(t, app.Save(target))

	other := core.NewRecord(patients)
	require.NoError(t, store.PatientToRecord(other, person.Patient{
		OwnerID: patienttest.StrangerID, FirstName: "Boris", LastName: "Novak", IsSelfRecord: true,
	}))
	require.NoError(t, app.Save(other))

	const pointsAtTargetID, pointsAtOtherID = "mkacctpoint0001", "mkacctpoint0002"

	seedAccountWithID(t, app, pointsAtTargetID, "points-at-target@example.test")
	seedAccountWithID(t, app, pointsAtOtherID, "points-at-other@example.test")

	pointsAtTarget, err := app.FindRecordById(store.AccountCollection, pointsAtTargetID)
	require.NoError(t, err)
	store.SetUserActivePatientID(pointsAtTarget, target.Id)
	require.NoError(t, app.Save(pointsAtTarget))

	pointsAtOther, err := app.FindRecordById(store.AccountCollection, pointsAtOtherID)
	require.NoError(t, err)
	store.SetUserActivePatientID(pointsAtOther, other.Id)
	require.NoError(t, app.Save(pointsAtOther))

	require.NoError(t, app.Delete(target))

	reloadedTarget, err := app.FindRecordById(store.AccountCollection, pointsAtTargetID)
	require.NoError(t, err, "the account itself must still exist (CascadeDelete: false)")
	assert.Empty(t, store.UserActivePatientID(reloadedTarget), "the pointer must be null once the patient it named is gone")

	reloadedOther, err := app.FindRecordById(store.AccountCollection, pointsAtOtherID)
	require.NoError(t, err)
	assert.Equal(t, other.Id, store.UserActivePatientID(reloadedOther),
		"an unrelated account's pointer must survive somebody else's patient being deleted")
}
