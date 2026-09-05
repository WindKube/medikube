package patient_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"

	_ "medikube/internal/store/migrations"
)

// FR-028, US4-1, SC-007: the chart's per-kind count equals the rows
// attributed to THIS patient and excludes every other patient's — the whole
// of what makes the count trustworthy rather than merely present.
func TestCountByPatientCountsOnlyThisPatientsRows(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	const owner = "mkchartowner001"
	seedAccountWithID(t, app, owner, "chart-owner@example.test")
	mine := seedRawPatient(t, app, owner, "Amara")
	theirs := seedRawPatient(t, app, owner, "Chidi")

	seedMedication(t, app, mine, "Amoxicillin")
	seedMedication(t, app, mine, "Ibuprofen")
	seedMedication(t, app, theirs, "Paracetamol")

	count, err := store.CountByPatient(t.Context(), app, kind.Medication.Collection(), mine)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	count, err = store.CountByPatient(t.Context(), app, kind.Medication.Collection(), theirs)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

// FR-030, US4-2: a patient with no rows at all still counts to zero rather
// than erroring.
func TestCountByPatientAnswersZeroForAPatientWithNoRows(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	const owner = "mkchartowner002"
	seedAccountWithID(t, app, owner, "chart-owner-2@example.test")
	empty := seedRawPatient(t, app, owner, "Bo")

	count, err := store.CountByPatient(t.Context(), app, kind.Medication.Collection(), empty)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func seedRawPatient(t *testing.T, app core.App, ownerID, firstName string) string {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	record := core.NewRecord(collection)
	record.Set("owner", ownerID)
	record.Set("first_name", firstName)
	record.Set("last_name", "Test")

	require.NoError(t, app.Save(record))

	return record.Id
}

func seedMedication(t *testing.T, app core.App, patientID, name string) {
	t.Helper()

	collection, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	record := core.NewRecord(collection)
	require.NoError(t, store.MedicationToRecord(record, clinical.Medication{
		PatientID: patientID, Name: name, Status: clinical.TherapyStatusActive,
	}))

	require.NoError(t, app.Save(record))
}
