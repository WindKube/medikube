package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagsCollectionHasAllFiveAPIRulesNil(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	require.NoError(t, AssertAPIRules(app))

	collection, err := app.FindCollectionByNameOrId(TagsCollection)
	require.NoError(t, err)
	assert.Nil(t, collection.ListRule)
	assert.Nil(t, collection.ViewRule)
	assert.Nil(t, collection.CreateRule)
	assert.Nil(t, collection.UpdateRule)
	assert.Nil(t, collection.DeleteRule)
}

func TestTagsHasAUniqueCaseInsensitiveIndexOnOwnerAndName(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(TagsCollection)
	require.NoError(t, err)

	declared := indexNamed(t, collection, tagsOwnerNameIndex)
	assert.Contains(t, declared, "UNIQUE")
	assert.Contains(t, declared, "LOWER("+tagFieldName+")")
}

func TestTagsDownRemovesTheCollectionCleanly(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	// search_index.tags, medications.tags and the tags fields belonging to
	// US1's three kinds and US5's two kinds are later migrations' relations
	// to this collection; unwinding in the real migration runner removes
	// them first, in reverse creation order. Reproduced here so this test
	// exercises tagsDown in the state it actually runs in.
	require.NoError(t, injuriesDown(app))
	require.NoError(t, immunizationsDown(app))
	require.NoError(t, insurancesDown(app))
	require.NoError(t, equipmentDown(app))
	require.NoError(t, emergencyContactsDown(app))
	require.NoError(t, conditionsDown(app))
	require.NoError(t, allergiesDown(app))
	require.NoError(t, medicationTagsDown(app))
	require.NoError(t, searchIndexDown(app))
	require.NoError(t, tagsDown(app))

	_, err := app.FindCollectionByNameOrId(TagsCollection)
	assert.Error(t, err)
}
