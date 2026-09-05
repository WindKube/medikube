package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	treatmentNameMax            = 300
	treatmentTypeMax            = 120
	treatmentDescriptionMax     = 5000
	treatmentFrequencyMax       = 100
	treatmentDosageMax          = 200
	treatmentExpectedOutcomeMax = 300
	treatmentNotesMax           = 5000
)

const (
	treatmentFieldPatient         = "patient"
	treatmentFieldName            = "name"
	treatmentFieldType            = "type"
	treatmentFieldSetting         = "setting"
	treatmentFieldDescription     = "description"
	treatmentFieldStartedOn       = "started_on"
	treatmentFieldEndedOn         = "ended_on"
	treatmentFieldFrequency       = "frequency"
	treatmentFieldDosage          = "dosage"
	treatmentFieldExpectedOutcome = "expected_outcome"
	treatmentFieldStatus          = "status"
	treatmentFieldPractitioner    = "practitioner"
	treatmentFieldFacility        = "facility"
	treatmentFieldNotes           = "notes"
	treatmentFieldTags            = "tags"
)

// Named after the collections they relate to, per data-model §4.5 (FR-028),
// so declared from the kind table rather than spelled a second time here.
var (
	treatmentFieldEncounters = kind.Encounter.Collection()
	treatmentFieldEquipment  = kind.Equipment.Collection()
)

func init() {
	register(treatmentsUp, treatmentsDown)
}

// treatmentsUp does not declare `condition` (see encounters.go's own note) or
// `lab_results` (phase 004's own migration). `encounters` and `equipment` are
// both wired: both target collections are this story's own.
func treatmentsUp(app core.App) error {
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

	encounters, err := app.FindCollectionByNameOrId(kind.Encounter.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Encounter.Collection(), err)
	}

	equipment, err := app.FindCollectionByNameOrId(kind.Equipment.Collection())
	if err != nil {
		return fmt.Errorf("finding %s: %w", kind.Equipment.Collection(), err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	name := kind.Treatment.Collection()
	collection := core.NewBaseCollection(name)
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name: treatmentFieldPatient, Required: true, CascadeDelete: true,
		MaxSelect: 1, CollectionId: patients.Id,
	})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldName, Required: true, Min: 2, Max: treatmentNameMax})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldType, Max: treatmentTypeMax})
	collection.Fields.Add(&core.SelectField{
		Name: treatmentFieldSetting, MaxSelect: 1, Values: enumValues(clinical.TreatmentSettings()),
	})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldDescription, Max: treatmentDescriptionMax})
	collection.Fields.Add(&core.DateField{Name: treatmentFieldStartedOn})
	collection.Fields.Add(&core.DateField{Name: treatmentFieldEndedOn})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldFrequency, Max: treatmentFrequencyMax})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldDosage, Max: treatmentDosageMax})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldExpectedOutcome, Max: treatmentExpectedOutcomeMax})
	collection.Fields.Add(&core.SelectField{
		Name: treatmentFieldStatus, MaxSelect: 1, Values: enumValues(clinical.TherapyStatuses()),
	})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentFieldPractitioner, MaxSelect: 1, CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentFieldFacility, MaxSelect: 1, CollectionId: facilities.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentFieldEncounters, MaxSelect: 0, CollectionId: encounters.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: treatmentFieldEquipment, MaxSelect: 0, CollectionId: equipment.Id,
	})
	collection.Fields.Add(&core.TextField{Name: treatmentFieldNotes, Max: treatmentNotesMax})
	collection.Fields.Add(&core.RelationField{Name: treatmentFieldTags, MaxSelect: 0, CollectionId: tags.Id})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	collection.AddIndex("idx_"+name+"_patient_started", false,
		treatmentFieldPatient+", "+treatmentFieldStartedOn+" DESC, id DESC", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func treatmentsDown(app core.App) error {
	return deleteCollection(app, kind.Treatment.Collection())
}
