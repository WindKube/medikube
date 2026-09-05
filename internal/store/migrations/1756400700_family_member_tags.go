package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// familyMemberFieldTags is data-model §0.8's universal `tags` relation,
// added here to family_member — the only one of the fourteen registered
// kinds whose own migration predates the tags collection's US7 rollout —
// the same follow-up shape as 1756400410_symptom_vitals_tags.go.
const familyMemberFieldTags = "tags"

func init() {
	register(familyMemberTagsUp, familyMemberTagsDown)
}

func familyMemberTagsUp(app core.App) error {
	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection, err := app.FindCollectionByNameOrId(kind.FamilyMember.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.FamilyMember.Collection(), err)
	}

	collection.Fields.Add(&core.RelationField{
		Name:         familyMemberFieldTags,
		MaxSelect:    unlimitedTags,
		CollectionId: tags.Id,
	})

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("saving %s: %w", kind.FamilyMember.Collection(), err)
	}

	return nil
}

func familyMemberTagsDown(app core.App) error {
	collection, err := app.FindCollectionByNameOrId(kind.FamilyMember.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.FamilyMember.Collection(), err)
	}

	collection.Fields.RemoveByName(familyMemberFieldTags)

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("saving %s: %w", kind.FamilyMember.Collection(), err)
	}

	return nil
}
