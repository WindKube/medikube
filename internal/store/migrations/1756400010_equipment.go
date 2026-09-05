package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// data-model §4.11's column bounds.
const (
	equipmentNameMin         = 2
	equipmentNameMax         = 200
	equipmentManufacturerMax = 200
	equipmentModelMax        = 100
	equipmentSerialMax       = 100
	equipmentInstructionsMax = 5000
	equipmentNotesMax        = 5000
)

// The columns of data-model §4.11, plus the two universal relations (§0.3,
// §0.8) and the two autodate columns (§0.2).
const (
	equipmentFieldPatient      = "patient"
	equipmentFieldName         = "name"
	equipmentFieldType         = "type"
	equipmentFieldManufacturer = "manufacturer"
	equipmentFieldModel        = "model"
	equipmentFieldSerial       = "serial"
	equipmentFieldPrescribedOn = "prescribed_on"
	equipmentFieldServicedOn   = "serviced_on"
	equipmentFieldServiceDueOn = "service_due_on"
	equipmentFieldInstructions = "instructions"
	equipmentFieldStatus       = "status"
	equipmentFieldSupplier     = "supplier"
	equipmentFieldPractitioner = "practitioner"
	equipmentFieldNotes        = "notes"
	equipmentFieldTags         = "tags"
)

func init() {
	register(equipmentUp, equipmentDown)
}

func equipmentUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	facilities, err := app.FindCollectionByNameOrId(facilitiesCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", facilitiesCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(kind.Equipment.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          equipmentFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     equipmentFieldName,
		Required: true,
		Min:      equipmentNameMin,
		Max:      equipmentNameMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      equipmentFieldType,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(clinical.EquipmentTypes()),
	})
	collection.Fields.Add(&core.TextField{Name: equipmentFieldManufacturer, Max: equipmentManufacturerMax})
	collection.Fields.Add(&core.TextField{Name: equipmentFieldModel, Max: equipmentModelMax})
	collection.Fields.Add(&core.TextField{Name: equipmentFieldSerial, Max: equipmentSerialMax})
	collection.Fields.Add(&core.DateField{Name: equipmentFieldPrescribedOn})
	collection.Fields.Add(&core.DateField{Name: equipmentFieldServicedOn})
	collection.Fields.Add(&core.DateField{Name: equipmentFieldServiceDueOn})
	collection.Fields.Add(&core.TextField{Name: equipmentFieldInstructions, Max: equipmentInstructionsMax})
	collection.Fields.Add(&core.SelectField{
		Name:      equipmentFieldStatus,
		MaxSelect: 1,
		Values:    enumValues(clinical.TherapyStatuses()),
	})
	collection.Fields.Add(&core.RelationField{
		Name:         equipmentFieldSupplier,
		MaxSelect:    1,
		CollectionId: facilities.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name:         equipmentFieldPractitioner,
		MaxSelect:    1,
		CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.TextField{Name: equipmentFieldNotes, Max: equipmentNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name:         equipmentFieldTags,
		MaxSelect:    0,
		CollectionId: tags.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Equipment.Collection()
	collection.AddIndex("idx_"+name+"_patient_presc", false, equipmentFieldPatient+", "+equipmentFieldPrescribedOn+", id", "")
	collection.AddIndex("idx_"+name+"_patient_due", false, equipmentFieldPatient+", "+equipmentFieldServiceDueOn, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func equipmentDown(app core.App) error {
	return deleteCollection(app, kind.Equipment.Collection())
}
