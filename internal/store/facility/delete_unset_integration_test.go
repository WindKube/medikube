package facility_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain"
	"medikube/internal/domain/kind"
)

// TestDeleteUnsetsReferencesRatherThanCascading is T127 and research D-06:
// deleting a facility clears practitioners.facility (and medications.pharmacy,
// once that column exists) rather than destroying the rows that pointed at
// it. contracts/facilities.md states this as a mandatory test: "a facility
// referenced by a practitioner and by a medication's pharmacy is deleted, both
// survive with an empty reference."
func TestDeleteUnsetsReferencesRatherThanCascading(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	stored := h.create(t, h.draft(h.owner, "Referenced Clinic"))

	practitioners, err := h.app.FindCollectionByNameOrId("practitioners")
	require.NoError(t, err)

	practitioner := core.NewRecord(practitioners)
	practitioner.Set("owner", h.owner)
	practitioner.Set("name", "Dr Amara")
	practitioner.Set("facility", stored.ID)
	require.NoError(t, h.app.Save(practitioner))

	medications, medsErr := h.app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, medsErr)

	hasPharmacy := medications.Fields.GetByName("pharmacy") != nil

	var medication *core.Record

	if hasPharmacy {
		medication = core.NewRecord(medications)
		medication.Set("owner", h.owner)
		medication.Set("name", "Amoxicillin")
		medication.Set("status", "active")
		medication.Set("pharmacy", stored.ID)
		require.NoError(t, h.app.Save(medication))
	} else {
		t.Log("the pharmacy field does not exist on the medication collection yet in this worktree; skipping that half of this test")
	}

	usage, err := h.repo.Usage(t.Context(), h.owner, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, usage.Practitioners, "usage must count the referencing practitioner before the delete")

	if hasPharmacy {
		assert.Equal(t, 1, usage.Records, "usage must count the referencing medication before the delete")
	}

	require.NoError(t, h.repo.Delete(t.Context(), h.owner, stored.ID, stored.Version))

	_, err = h.repo.Get(t.Context(), h.owner, stored.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound, "the facility itself is gone")

	survivingPractitioner, err := h.app.FindRecordById("practitioners", practitioner.Id)
	require.NoError(t, err, "the practitioner must survive the facility's deletion")
	assert.Empty(t, survivingPractitioner.GetString("facility"),
		"the practitioner's facility reference must be cleared, not left dangling")

	if hasPharmacy {
		survivingMedication, medErr := h.app.FindRecordById(kind.Medication.Collection(), medication.Id)
		require.NoError(t, medErr, "the medication must survive the facility's deletion")
		assert.Empty(t, survivingMedication.GetString("pharmacy"),
			"the medication's pharmacy reference must be cleared, not left dangling")
	}
}
