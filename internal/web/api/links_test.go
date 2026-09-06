package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
	"medikube/internal/testsupport/seed"
)

type referencesEnvelope struct {
	References struct {
		Total int `json:"total"`
	} `json:"references"`
}

// TestPatchingAllergyMedicationsIsVisibleFromTheMedicationSideAndClearsBoth is
// T150/FR-055/FR-056/FR-006: a medication-link editor on either end issues the
// same PATCH of the owning record's own `medications` field — there is no
// second write path for "the other side" to use — so this proves that one op
// is enough: creating the link is immediately visible reading the medication's
// own back-relation count, and clearing it clears both records' view of the
// link at once, with no dangling copy on either.
func TestPatchingAllergyMedicationsIsVisibleFromTheMedicationSideAndClearsBoth(t *testing.T) {
	t.Parallel()

	owner := newCaller(t)

	allergyTarget := allergyURL(seed.CriticalAllergyID)
	medicationTarget := recordURL(seed.AllergyLinkedMedicationID)
	medicationsField := kind.Medication.Collection()

	// This same medication is also seeded as the symptom's own caused_by
	// role and as the course medication join's medication, so its baseline
	// reference count is 3 rather than 1 — the allergy link is one of them.
	before := owner.get(medicationTarget)
	require.Equal(t, http.StatusOK, before.Status, before.Body)

	var beforeBody referencesEnvelope
	before.decode(t, &beforeBody)
	require.Equal(t, 3, beforeBody.References.Total, "the seeded fixtures already link this medication three ways")

	current := owner.get(allergyTarget)
	require.Equal(t, http.StatusOK, current.Status, current.Body)

	cleared := owner.patch(allergyTarget, `{"`+medicationsField+`":[]}`, current.etag(t))
	require.Equal(t, http.StatusOK, cleared.Status, cleared.Body)

	afterClear := owner.get(medicationTarget)
	require.Equal(t, http.StatusOK, afterClear.Status, afterClear.Body)

	var afterClearBody referencesEnvelope
	afterClear.decode(t, &afterClearBody)
	assert.Equal(t, 2, afterClearBody.References.Total,
		"removing the allergy's own field clears the medication's back-relation to it, and only that one")

	restored := owner.patch(allergyTarget, `{"`+medicationsField+`":["`+seed.AllergyLinkedMedicationID+`"]}`, cleared.etag(t))
	require.Equal(t, http.StatusOK, restored.Status, restored.Body)

	afterRestore := owner.get(medicationTarget)
	require.Equal(t, http.StatusOK, afterRestore.Status, afterRestore.Body)

	var afterRestoreBody referencesEnvelope
	afterRestore.decode(t, &afterRestoreBody)
	assert.Equal(t, 3, afterRestoreBody.References.Total, "creating the link is visible from the medication side")
}
