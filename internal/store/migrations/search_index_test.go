package migrations

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

func TestSearchIndexHasAllFiveAPIRulesNil(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)
	require.NoError(t, AssertAPIRules(app))

	collection, err := app.FindCollectionByNameOrId(SearchIndexCollection)
	require.NoError(t, err)
	assert.Nil(t, collection.ListRule)
	assert.Nil(t, collection.ViewRule)
	assert.Nil(t, collection.CreateRule)
	assert.Nil(t, collection.UpdateRule)
	assert.Nil(t, collection.DeleteRule)
}

func TestSearchIndexPatientRelationCascades(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(SearchIndexCollection)
	require.NoError(t, err)

	relation, err := relationField(collection, searchFieldPatient)
	require.NoError(t, err)
	assert.True(t, relation.Required)
	assert.True(t, relation.CascadeDelete)
	assert.Equal(t, 1, relation.MaxSelect)
}

func TestSearchIndexTagsRelationAllowsMoreThanOneTag(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(SearchIndexCollection)
	require.NoError(t, err)

	relation, err := relationField(collection, searchFieldTags)
	require.NoError(t, err)
	assert.Equal(t, unlimitedTags, relation.MaxSelect, "any number of tags: PocketBase reads MaxSelect<=1 as single-select, so this cannot be 0")
	assert.False(t, relation.Required)
}

func TestSearchIndexPublishesEveryRegisteredKind(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(SearchIndexCollection)
	require.NoError(t, err)

	field := collection.Fields.GetByName(searchFieldKind)
	require.NotNil(t, field)

	selectField, ok := field.(*core.SelectField)
	require.True(t, ok)
	assert.ElementsMatch(t, kindSegments(), selectField.Values)
	assert.Equal(t, 1, selectField.MaxSelect)
	assert.True(t, selectField.Required)
}

func TestSearchIndexIndexes(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(SearchIndexCollection)
	require.NoError(t, err)

	unique := indexNamed(t, collection, searchRecordIndex)
	assert.Contains(t, unique, "UNIQUE")
	assert.Contains(t, unique, searchFieldKind)
	assert.Contains(t, unique, searchFieldRecordID)

	patientKind := indexNamed(t, collection, searchPatientKindIndex)
	assert.Contains(t, patientKind, searchFieldPatient)
	assert.Contains(t, patientKind, searchFieldKind)
	assert.Contains(t, patientKind, searchFieldOccurredOn)
	assert.Contains(t, patientKind, "id")
}

func TestSearchIndexDownRemovesTheCollectionCleanly(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, searchIndexDown(app))

	_, err := app.FindCollectionByNameOrId(SearchIndexCollection)
	assert.Error(t, err)
}

func TestKindSegmentsMatchesTheRegistry(t *testing.T) {
	t.Parallel()

	assert.Len(t, kindSegments(), len(kind.Kinds()))
	assert.Len(t, kindSegments(), 14)
}
