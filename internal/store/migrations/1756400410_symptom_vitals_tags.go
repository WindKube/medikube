package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// symptomVitalsFieldTags is data-model §0.8's universal `tags` relation,
// added here to the two kinds whose own migration predates the tags
// collection's US7 rollout — the same follow-up shape as
// 1756300020_medication_tags.go.
const symptomVitalsFieldTags = "tags"

func init() {
	register(symptomVitalsTagsUp, symptomVitalsTagsDown)
}

func symptomVitalsTagsUp(app core.App) error {
	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	for _, k := range []kind.Kind{kind.Symptom, kind.Vitals} {
		collection, findErr := app.FindCollectionByNameOrId(k.Collection())
		if findErr != nil {
			return fmt.Errorf("finding %s: %w", k.Collection(), findErr)
		}

		collection.Fields.Add(&core.RelationField{
			Name:         symptomVitalsFieldTags,
			MaxSelect:    0,
			CollectionId: tags.Id,
		})

		if saveErr := app.Save(collection); saveErr != nil {
			return fmt.Errorf("saving %s: %w", k.Collection(), saveErr)
		}
	}

	return nil
}

func symptomVitalsTagsDown(app core.App) error {
	for _, k := range []kind.Kind{kind.Symptom, kind.Vitals} {
		collection, findErr := app.FindCollectionByNameOrId(k.Collection())
		if findErr != nil {
			return fmt.Errorf("finding %s: %w", k.Collection(), findErr)
		}

		collection.Fields.RemoveByName(symptomVitalsFieldTags)

		if saveErr := app.Save(collection); saveErr != nil {
			return fmt.Errorf("saving %s: %w", k.Collection(), saveErr)
		}
	}

	return nil
}
