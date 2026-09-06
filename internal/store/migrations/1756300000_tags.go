package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// TagsCollection is the account-owned tag vocabulary (data-model §5.1). Not a
// kind.Kind — a tag is not a clinical record, it is what every clinical
// record's own `tags` relation points at.
const TagsCollection = "tags"

const (
	tagFieldOwner = "owner"
	tagFieldName  = "name"
	tagFieldColor = "color"
)

const (
	tagNameMin = 1
	tagNameMax = 40
)

const tagColorPattern = `^#[0-9a-fA-F]{6}$`

const tagsOwnerNameIndex = "uniq_tags_owner_name"

// unlimitedTags is every carrier's own `tags` RelationField's MaxSelect
// (FR-064: "any number of tags"). PocketBase's RelationField.IsMultiple
// treats MaxSelect <= 1 as a single-value relation — 0 does NOT mean
// unlimited, it means "at most one" — so "any number" has to name an actual
// ceiling. There is no requirement capping how many tags one record may
// carry, so this is generous rather than exact.
const unlimitedTags = 999

func init() {
	register(tagsUp, tagsDown)
}

func tagsUp(app core.App) error {
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", usersCollection, err)
	}

	collection := core.NewBaseCollection(TagsCollection)
	lockRules(collection)

	// Tags belong to the account, not to a patient (FR-062, FR-005 of tag
	// privacy): a shared installation never discloses one household's tags to
	// another.
	collection.Fields.Add(&core.RelationField{
		Name:          tagFieldOwner,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  users.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     tagFieldName,
		Required: true,
		Min:      tagNameMin,
		Max:      tagNameMax,
	})
	collection.Fields.Add(&core.TextField{
		Name:    tagFieldColor,
		Pattern: tagColorPattern,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	// FR-063: case-insensitive uniqueness per owner.
	collection.AddIndex(tagsOwnerNameIndex, true, tagFieldOwner+", LOWER("+tagFieldName+")", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", TagsCollection, err)
	}

	return nil
}

func tagsDown(app core.App) error {
	return deleteCollection(app, TagsCollection)
}
