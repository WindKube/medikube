package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/kind"
)

// linksFieldMedications is the `medications` multi-relation data-model §4.1
// and §4.2 add to allergies and conditions (FR-017, FR-021). It is deferred to
// this migration, not each kind's own, because splitting the link fields out
// is what keeps the dependency graph acyclic (data-model §8, migration 17).
// Its value is the medications collection's own name, derived rather than
// spelled a second time (research D-05) — the field is named after its
// target by data-model design.
var linksFieldMedications = kind.Medication.Collection()

// symptomFieldTreatedByMedications and symptomFieldCausedByMedications are
// FR-032's two distinct medication roles on a symptom: one treats it, the
// other is suspected of causing it. Never one field with a flag — the two are
// independent sets and a medication may belong to both. Built from the
// collection name rather than spelled whole, for the same reason.
var (
	symptomFieldTreatedByMedications = "treated_by_" + kind.Medication.Collection()
	symptomFieldCausedByMedications  = "caused_by_" + kind.Medication.Collection()
)

// maxLinkedMedications bounds every medication-relation field this migration
// touches, including injuries.medication_ids (US4, migration 12): PocketBase's
// own RelationField.IsMultiple is `MaxSelect > 1`, not `!= 1` — a field left at
// MaxSelect:0 stores one id, not a set, which would silently break FR-056's
// whole replace-set contract (and FR-042's, for injuries). 100 mirrors
// store.MaxLimit: a person is not going to link more medications to one
// record than would fit a single page of their own medications list.
const maxLinkedMedications = 100

func init() {
	register(linksMedicationsUp, linksMedicationsDown)
}

func linksMedicationsUp(app core.App) error {
	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	for _, target := range []kind.Kind{kind.Allergy, kind.Condition} {
		name := target.Collection()

		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("finding %s: %w", name, err)
		}

		collection.Fields.Add(&core.RelationField{
			Name: linksFieldMedications, MaxSelect: maxLinkedMedications, CollectionId: medications.Id,
		})

		if err := app.Save(collection); err != nil {
			return fmt.Errorf("adding %s.%s: %w", name, linksFieldMedications, err)
		}
	}

	symptoms, err := app.FindCollectionByNameOrId(kind.Symptom.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Symptom.Collection(), err)
	}

	symptoms.Fields.Add(&core.RelationField{
		Name: symptomFieldTreatedByMedications, MaxSelect: maxLinkedMedications, CollectionId: medications.Id,
	})
	symptoms.Fields.Add(&core.RelationField{
		Name: symptomFieldCausedByMedications, MaxSelect: maxLinkedMedications, CollectionId: medications.Id,
	})

	if err := app.Save(symptoms); err != nil {
		return fmt.Errorf("adding the medication role fields to %s: %w", kind.Symptom.Collection(), err)
	}

	if err := setInjuryMedicationsMaxSelect(app, maxLinkedMedications); err != nil {
		return err
	}

	return nil
}

// setInjuryMedicationsMaxSelect raises (or, in linksMedicationsDown, lowers
// back) injuries.medication_ids's cap in place: the field itself was added by
// migration 12 (US4), and this migration is where every other
// medication-relation field's MaxSelect bug is fixed, so it is fixed here too
// rather than by editing an already-applied migration.
func setInjuryMedicationsMaxSelect(app core.App, maxSelect int) error {
	injuries, err := app.FindCollectionByNameOrId(kind.Injury.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Injury.Collection(), err)
	}

	relation, err := relationField(injuries, injuryFieldMedications)
	if err != nil {
		return err
	}

	relation.MaxSelect = maxSelect

	if err := app.Save(injuries); err != nil {
		return fmt.Errorf("setting %s.%s's MaxSelect: %w", kind.Injury.Collection(), injuryFieldMedications, err)
	}

	return nil
}

func linksMedicationsDown(app core.App) error {
	if err := setInjuryMedicationsMaxSelect(app, 0); err != nil {
		return err
	}

	symptoms, err := app.FindCollectionByNameOrId(kind.Symptom.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Symptom.Collection(), err)
	}

	symptoms.Fields.RemoveByName(symptomFieldCausedByMedications)
	symptoms.Fields.RemoveByName(symptomFieldTreatedByMedications)

	if err := app.Save(symptoms); err != nil {
		return fmt.Errorf("removing the medication role fields from %s: %w", kind.Symptom.Collection(), err)
	}

	for _, target := range []kind.Kind{kind.Condition, kind.Allergy} {
		name := target.Collection()

		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return fmt.Errorf("finding %s: %w", name, err)
		}

		collection.Fields.RemoveByName(linksFieldMedications)

		if err := app.Save(collection); err != nil {
			return fmt.Errorf("removing %s.%s: %w", name, linksFieldMedications, err)
		}
	}

	return nil
}
