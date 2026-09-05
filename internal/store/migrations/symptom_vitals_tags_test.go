package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

func TestSymptomsAndVitalsCarryATagsRelation(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	for _, k := range []kind.Kind{kind.Symptom, kind.Vitals} {
		collection, err := app.FindCollectionByNameOrId(k.Collection())
		require.NoError(t, err)

		relation, err := relationField(collection, symptomVitalsFieldTags)
		require.NoError(t, err)
		assert.Equal(t, 0, relation.MaxSelect, "any number of tags")
		assert.False(t, relation.Required)
	}
}

func TestSymptomVitalsTagsDownRemovesTheField(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, symptomVitalsTagsDown(app))

	for _, k := range []kind.Kind{kind.Symptom, kind.Vitals} {
		collection, err := app.FindCollectionByNameOrId(k.Collection())
		require.NoError(t, err)
		assert.Nil(t, collection.Fields.GetByName(symptomVitalsFieldTags))
	}
}
