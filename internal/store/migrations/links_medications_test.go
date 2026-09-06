package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

// T144: migration 17 adds the `medications` multi-relation to allergies and
// conditions (FR-017, FR-021) and the two distinct medication roles to
// symptoms (FR-032). None of the four is MaxSelect 1, so none belongs in the
// single-relation cascade matrix (assertions_test.go) — this is where they
// are asserted instead.
func TestLinksMedicationsAddsTheFourMultiRelationFields(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	cases := []struct {
		collection string
		field      string
	}{
		{kind.Allergy.Collection(), linksFieldMedications},
		{kind.Condition.Collection(), linksFieldMedications},
		{kind.Symptom.Collection(), symptomFieldTreatedByMedications},
		{kind.Symptom.Collection(), symptomFieldCausedByMedications},
	}

	for _, tt := range cases {
		t.Run(tt.collection+"."+tt.field, func(t *testing.T) {
			t.Parallel()

			collection, err := app.FindCollectionByNameOrId(tt.collection)
			require.NoError(t, err)

			relation, err := relationField(collection, tt.field)
			require.NoError(t, err)

			assert.Equal(t, maxLinkedMedications, relation.MaxSelect,
				"MaxSelect must be > 1 or PocketBase treats the field as single-valued (RelationField.IsMultiple)")
			assert.False(t, relation.Required)
			assert.False(t, relation.CascadeDelete, "deleting a medication must leave the referencing record intact (FR-058)")
			assert.Equal(t, medications.Id, relation.CollectionId)
		})
	}
}

// T144: this migration also raises injuries.medication_ids (US4, migration
// 12) to the same cap — the field was already there, but at the same
// MaxSelect:0 that turned out to mean "single id" rather than "a set".
func TestLinksMedicationsRaisesInjuryMedicationsMaxSelect(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	injuries, err := app.FindCollectionByNameOrId(kind.Injury.Collection())
	require.NoError(t, err)

	relation, err := relationField(injuries, injuryFieldMedications)
	require.NoError(t, err)

	assert.Equal(t, maxLinkedMedications, relation.MaxSelect)
}

func TestLinksMedicationsDownRemovesAllFourFields(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, linksMedicationsDown(app))

	cases := []struct {
		collection string
		field      string
	}{
		{kind.Allergy.Collection(), linksFieldMedications},
		{kind.Condition.Collection(), linksFieldMedications},
		{kind.Symptom.Collection(), symptomFieldTreatedByMedications},
		{kind.Symptom.Collection(), symptomFieldCausedByMedications},
	}

	for _, tt := range cases {
		collection, err := app.FindCollectionByNameOrId(tt.collection)
		require.NoError(t, err)

		assert.Nil(t, collection.Fields.GetByName(tt.field))
	}

	injuries, err := app.FindCollectionByNameOrId(kind.Injury.Collection())
	require.NoError(t, err)

	relation, err := relationField(injuries, injuryFieldMedications)
	require.NoError(t, err)
	assert.Equal(t, 0, relation.MaxSelect, "down must restore the field to the state migration 12 left it in")

	require.NoError(t, linksMedicationsUp(app))
}
