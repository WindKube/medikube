package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"medikube/internal/domain/kind"
)

func TestFamilyMemberCarriesATagsRelation(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	collection, err := app.FindCollectionByNameOrId(kind.FamilyMember.Collection())
	require.NoError(t, err)

	relation, err := relationField(collection, familyMemberFieldTags)
	require.NoError(t, err)
	assert.Equal(t, unlimitedTags, relation.MaxSelect, "any number of tags: PocketBase reads MaxSelect<=1 as single-select, so this cannot be 0")
	assert.False(t, relation.Required)
}

func TestFamilyMemberTagsDownRemovesTheField(t *testing.T) {
	t.Parallel()

	app := newTestApp(t)

	require.NoError(t, familyMemberTagsDown(app))

	collection, err := app.FindCollectionByNameOrId(kind.FamilyMember.Collection())
	require.NoError(t, err)
	assert.Nil(t, collection.Fields.GetByName(familyMemberFieldTags))
}
