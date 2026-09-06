package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	injuryNameMin          = 2
	injuryNameMax          = 300
	injuryBodyPartMax      = 100
	injuryMechanismMax     = 500
	injuryRecoveryNotesMax = 2000
)

const (
	injuryFieldPatient       = "patient"
	injuryFieldPractitioner  = "practitioner"
	injuryFieldName          = "name"
	injuryFieldType          = "type"
	injuryFieldBodyPart      = "body_part"
	injuryFieldLaterality    = "laterality"
	injuryFieldOccurredOn    = "occurred_on"
	injuryFieldMechanism     = "mechanism"
	injuryFieldSeverity      = "severity"
	injuryFieldStatus        = "status"
	injuryFieldRecoveryNotes = "recovery_notes"
	// injuryFieldMedications is the one of FR-042's four link fields this
	// phase stores as a real relation. conditions, procedures and treatments
	// are kinds sibling phase-003 branches add (data-model.md §8's migration
	// 14 depends on them); linking to them is deferred to the migration that
	// adds those collections, the same way medications' own tags field was
	// added retroactively rather than at its birth (1756300020_medication_
	// tags.go).
	injuryFieldMedications = "medication_ids"
	injuryFieldTags        = "tags"
)

func init() {
	register(injuriesUp, injuriesDown)
}

func injuriesUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	medications, err := app.FindCollectionByNameOrId(kind.Medication.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Medication.Collection(), err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(kind.Injury.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          injuryFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: injuryFieldPractitioner, MaxSelect: 1, CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name: injuryFieldName, Required: true, Min: injuryNameMin, Max: injuryNameMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name: injuryFieldType, MaxSelect: 1, Values: enumValues(clinical.InjuryTypes()),
	})
	collection.Fields.Add(&core.TextField{
		Name: injuryFieldBodyPart, Required: true, Max: injuryBodyPartMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name: injuryFieldLaterality, MaxSelect: 1, Values: enumValues(clinical.Lateralities()),
	})
	collection.Fields.Add(&core.DateField{Name: injuryFieldOccurredOn})
	collection.Fields.Add(&core.TextField{Name: injuryFieldMechanism, Max: injuryMechanismMax})
	collection.Fields.Add(&core.SelectField{
		Name: injuryFieldSeverity, MaxSelect: 1, Values: enumValues(clinical.Severities()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: injuryFieldStatus, Required: true, MaxSelect: 1, Values: enumValues(clinical.ConditionStatuses()),
	})
	collection.Fields.Add(&core.TextField{Name: injuryFieldRecoveryNotes, Max: injuryRecoveryNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name: injuryFieldMedications, MaxSelect: 0, CollectionId: medications.Id,
	})
	collection.Fields.Add(&core.RelationField{Name: injuryFieldTags, MaxSelect: unlimitedTags, CollectionId: tags.Id})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Injury.Collection()
	collection.AddIndex("idx_"+name+"_patient_date", false,
		injuryFieldPatient+", "+injuryFieldOccurredOn+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_patient_status", false,
		injuryFieldPatient+", "+injuryFieldStatus, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func injuriesDown(app core.App) error {
	return deleteCollection(app, kind.Injury.Collection())
}
