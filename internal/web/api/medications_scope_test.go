package api_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport"
)

// T076, contracts/medications-rescope.md's "Lists" mandatory tests: `?patient=`
// scopes a list to exactly the patient named, never to the account that owns
// both patients (FR-023, US2-2, SC-007), its absence is a 400 that names no
// patient (FR-016), and naming another account's patient is a 404 (US2-5).
//
// The shared committed fixture deliberately keeps every one of Account A's
// medications on her own self-record (internal/testsupport/seed), so that its
// row counts stay a stable anchor for every other test that counts them. This
// test therefore builds its own second patient and her own two medications
// directly against the database, rather than perturb that fixture.
func newMedicationOn(t *testing.T, c *caller, patientID, name string) string {
	t.Helper()

	collection, err := c.app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("patient", patientID)
	record.Set("name", name)
	record.Set("status", "active")
	require.NoError(t, c.app.Save(record))

	return record.Id
}

func TestAListForOnePatientHoldsOnlyThatPatientsMedications(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	patientX := newPatientFor(t, caller, testsupport.AccountAID)
	patientY := newPatientFor(t, caller, testsupport.AccountAID)

	for _, name := range []string{"Amoxicillin", "Ibuprofen", "Paracetamol"} {
		newMedicationOn(t, caller, patientX, name)
	}

	for _, name := range []string{"Metformin", "Lisinopril"} {
		newMedicationOn(t, caller, patientY, name)
	}

	x := caller.get(collectionURL() + "?patient=" + patientX)
	require.Equal(t, http.StatusOK, x.Status, x.Body)

	y := caller.get(collectionURL() + "?patient=" + patientY)
	require.Equal(t, http.StatusOK, y.Status, y.Body)

	assert.Len(t, x.list(t).Items, 3, "patient X's list bled rows from patient Y or the account")
	assert.Len(t, y.list(t).Items, 2, "patient Y's list bled rows from patient X or the account")
}

func TestAListWithoutAPatientIsRefusedAndNamesNoPatient(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.get(collectionURL())
	require.Equal(t, http.StatusBadRequest, answer.Status, answer.Body)

	envelope := answer.envelope(t)
	assert.Equal(t, "patient_required", envelope.Error.Code)
	assert.NotContains(t, answer.Body, testsupport.AccountAPatientSelfID,
		"a refusal for want of a patient named one anyway")
}

func TestAListNamingAnotherAccountsPatientIsAMiss(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	answer := caller.get(collectionURL() + "?patient=" + testsupport.AccountBPatientSelfID)

	require.Equal(t, http.StatusNotFound, answer.Status, answer.Body)
	assert.Equal(t, "not_found", answer.envelope(t).Error.Code)
}

// T077, FR-024, US2-4: a patch cannot re-file a medication onto a different
// patient because the patch DTO has no `patient` member at all — an unknown
// field is refused by the decoder before any business rule runs, and the
// stored record is unchanged.
func TestAPatchNamingAPatientIsRefusedAndTheRecordIsUnchanged(t *testing.T) {
	t.Parallel()

	caller := newCaller(t)

	before := caller.get(recordURL(testsupport.SingleDayMedicationID))
	require.Equal(t, http.StatusOK, before.Status, before.Body)

	answer := caller.patch(recordURL(testsupport.SingleDayMedicationID),
		`{"patient":"`+testsupport.AccountAPatientChildID+`"}`, before.etag(t))

	require.Equal(t, http.StatusUnprocessableEntity, answer.Status, answer.Body)
	assert.Contains(t, answer.envelope(t).fieldCodes(), [2]string{"patient", "unknown_field"})

	after := caller.get(recordURL(testsupport.SingleDayMedicationID))
	require.Equal(t, http.StatusOK, after.Status, after.Body)
	assert.Equal(t, before.Body, after.Body, "a refused patch changed the stored record anyway")
}
