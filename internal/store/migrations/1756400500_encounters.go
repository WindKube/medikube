package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	encounterReasonMax   = 300
	encounterFreeTextMax = 5000
	encounterFollowUpMax = 2000
	encounterNotesMax    = 5000
)

const (
	encounterFieldPatient      = "patient"
	encounterFieldReason       = "reason"
	encounterFieldOccurredOn   = "occurred_on"
	encounterFieldVisitType    = "visit_type"
	encounterFieldPriority     = "priority"
	encounterFieldAssessment   = "assessment"
	encounterFieldPlan         = "plan"
	encounterFieldFollowUp     = "follow_up"
	encounterFieldDurationMin  = "duration_minutes"
	encounterFieldPractitioner = "practitioner"
	encounterFieldFacility     = "facility"
	encounterFieldNotes        = "notes"
	encounterFieldTags         = "tags"
)

func init() {
	register(encountersUp, encountersDown)
}

// encountersUp does not declare `condition` (added by
// 1756400530_care_conditions.go once US1's conditions collection exists) or
// `lab_results` (phase 004's own migration).
func encountersUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	facilities, err := app.FindCollectionByNameOrId(facilitiesCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", facilitiesCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	name := kind.Encounter.Collection()
	collection := core.NewBaseCollection(name)
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name: encounterFieldPatient, Required: true, CascadeDelete: true,
		MaxSelect: 1, CollectionId: patients.Id,
	})
	collection.Fields.Add(&core.TextField{Name: encounterFieldReason, Required: true, Min: 1, Max: encounterReasonMax})
	collection.Fields.Add(&core.DateField{Name: encounterFieldOccurredOn, Required: true})
	collection.Fields.Add(&core.SelectField{
		Name: encounterFieldVisitType, MaxSelect: 1, Values: enumValues(clinical.VisitTypes()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: encounterFieldPriority, MaxSelect: 1, Values: enumValues(clinical.VisitPriorities()),
	})
	collection.Fields.Add(&core.TextField{Name: encounterFieldAssessment, Max: encounterFreeTextMax})
	collection.Fields.Add(&core.TextField{Name: encounterFieldPlan, Max: encounterFreeTextMax})
	collection.Fields.Add(&core.TextField{Name: encounterFieldFollowUp, Max: encounterFollowUpMax})
	collection.Fields.Add(&core.NumberField{Name: encounterFieldDurationMin, Min: types.Pointer(0.0)})
	collection.Fields.Add(&core.RelationField{
		Name: encounterFieldPractitioner, MaxSelect: 1, CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: encounterFieldFacility, MaxSelect: 1, CollectionId: facilities.Id,
	})
	collection.Fields.Add(&core.TextField{Name: encounterFieldNotes, Max: encounterNotesMax})
	collection.Fields.Add(&core.RelationField{Name: encounterFieldTags, MaxSelect: 0, CollectionId: tags.Id})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	collection.AddIndex("idx_"+name+"_patient_date", false,
		encounterFieldPatient+", "+encounterFieldOccurredOn+" DESC, id DESC", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func encountersDown(app core.App) error {
	return deleteCollection(app, kind.Encounter.Collection())
}
