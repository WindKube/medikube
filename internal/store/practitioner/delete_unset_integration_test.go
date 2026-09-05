package practitioner_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/directory"
	"medikube/internal/domain/kind"
)

// T126, research D-06: deleting a practitioner clears patients.primary_practitioner
// and medications.practitioner rather than destroying the rows that pointed at it.
func TestDeletingAPractitionerClearsEveryReference(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored, err := h.repo.Create(t.Context(), directory.Practitioner{OwnerID: h.owner, Name: "Dr Amara"})
	require.NoError(t, err)

	patients, err := h.app.FindCollectionByNameOrId("patients")
	require.NoError(t, err)

	patient := core.NewRecord(patients)
	patient.Set("owner", h.owner)
	patient.Set("first_name", "Amara")
	patient.Set("primary_practitioner", stored.ID)
	require.NoError(t, h.app.Save(patient))

	medications, err := h.app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	medication := core.NewRecord(medications)
	medication.Set("owner", h.owner)
	medication.Set("patient", patient.Id)
	medication.Set("name", "Amoxicillin")
	medication.Set("status", "active")
	medication.Set("practitioner", stored.ID)
	require.NoError(t, h.app.Save(medication))

	usage, err := h.repo.Usage(t.Context(), h.owner, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, usage.Patients)
	assert.Equal(t, 1, usage.Records)

	require.NoError(t, h.repo.Delete(t.Context(), h.owner, stored.ID, stored.Version))

	_, err = h.repo.Get(t.Context(), h.owner, stored.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	survivingPatient, err := h.app.FindRecordById("patients", patient.Id)
	require.NoError(t, err, "the patient must survive the practitioner's deletion")
	assert.Empty(t, survivingPatient.GetString("primary_practitioner"))

	survivingMedication, err := h.app.FindRecordById(kind.Medication.Collection(), medication.Id)
	require.NoError(t, err, "the medication must survive the practitioner's deletion")
	assert.Empty(t, survivingMedication.GetString("practitioner"))
}
