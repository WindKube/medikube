package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// medicationFieldTags is data-model §0.8's universal `tags` relation, added
// here to medications retroactively (FR-064): any number of tags, including on
// medications recorded in phase 001.
const medicationFieldTags = "tags"

func init() {
	register(medicationTagsUp, medicationTagsDown)
}

func medicationTagsUp(app core.App) error {
	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	medications.Fields.Add(&core.RelationField{
		Name:         medicationFieldTags,
		MaxSelect:    unlimitedTags,
		CollectionId: tags.Id,
	})

	if err := app.Save(medications); err != nil {
		return fmt.Errorf("saving %s: %w", kind.Medication.Collection(), err)
	}

	return nil
}

func medicationTagsDown(app core.App) error {
	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	medications.Fields.RemoveByName(medicationFieldTags)

	if err := app.Save(medications); err != nil {
		return fmt.Errorf("saving %s: %w", kind.Medication.Collection(), err)
	}

	return nil
}
