package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

func TestMedicationsCarriesATagsRelation(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)

	relation, err := relationField(medications, medicationFieldTags)
	require.NoError(t, err)
	assert.Equal(t, unlimitedTags, relation.MaxSelect, "any number of tags: PocketBase reads MaxSelect<=1 as single-select, so this cannot be 0")
	assert.False(t, relation.Required)
}

func TestMedicationTagsDownRemovesTheField(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, medicationTagsDown(app))

	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	require.NoError(t, err)
	assert.Nil(t, medications.Fields.GetByName(medicationFieldTags))
}
