package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// careFieldCondition is data-model §4.3/§4.4/§4.5's `condition` relation on
// encounters, procedures and treatments (FR-021/FR-022): deferred out of
// each kind's own migration (T061-T063) because US1's `conditions` collection
// did not exist on this branch's base, and added here now that it does.
const careFieldCondition = "condition"

func init() {
	register(careConditionsUp, careConditionsDown)
}

// careConditionsUp adds one optional, non-cascading relation to each of the
// three collections T075-T077 already created, in the same order. It never
// touches assessment/plan (FR-023's separation is a shape fact, not a value
// this migration could reach).
func careConditionsUp(app core.App) error {
	conditions, err := app.FindCollectionByNameOrId(kind.Condition.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Condition.Collection(), err)
	}

	for _, target := range []kind.Kind{kind.Encounter, kind.Procedure, kind.Treatment} {
		name := target.Collection()

		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("finding %s: %w", name, err)
		}

		collection.Fields.Add(&core.RelationField{
			Name: careFieldCondition, MaxSelect: 1, CollectionId: conditions.Id,
		})

		collection.AddIndex("idx_"+name+"_condition", false, careFieldCondition, "")

		if err := app.Save(collection); err != nil {
			return fmt.Errorf("adding %s.%s: %w", name, careFieldCondition, err)
		}
	}

	return nil
}

func careConditionsDown(app core.App) error {
	for _, target := range []kind.Kind{kind.Treatment, kind.Procedure, kind.Encounter} {
		name := target.Collection()

		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("finding %s: %w", name, err)
		}

		collection.RemoveIndex("idx_" + name + "_condition")
		collection.Fields.RemoveByName(careFieldCondition)

		if err := app.Save(collection); err != nil {
			return fmt.Errorf("removing %s.%s: %w", name, careFieldCondition, err)
		}
	}

	return nil
}
