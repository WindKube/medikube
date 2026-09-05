package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	procedureNameMax            = 300
	procedureCodeMax            = 50
	procedureDescriptionMax     = 5000
	procedureComplicationsMax   = 500
	procedureAnesthesiaNotesMax = 2000
	procedureNotesMax           = 5000
)

const (
	procedureFieldPatient         = "patient"
	procedureFieldName            = "name"
	procedureFieldType            = "type"
	procedureFieldCode            = "code"
	procedureFieldDescription     = "description"
	procedureFieldOccurredOn      = "occurred_on"
	procedureFieldStatus          = "status"
	procedureFieldOutcome         = "outcome"
	procedureFieldSetting         = "setting"
	procedureFieldComplications   = "complications"
	procedureFieldDurationMin     = "duration_minutes"
	procedureFieldAnesthesia      = "anesthesia"
	procedureFieldAnesthesiaNotes = "anesthesia_notes"
	procedureFieldPractitioner    = "practitioner"
	procedureFieldFacility        = "facility"
	procedureFieldNotes           = "notes"
	procedureFieldTags            = "tags"
)

func init() {
	register(proceduresUp, proceduresDown)
}

// proceduresUp does not declare `condition` (see encounters.go's own note).
func proceduresUp(app core.App) error {
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

	name := kind.Procedure.Collection()
	collection := core.NewBaseCollection(name)
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name: procedureFieldPatient, Required: true, CascadeDelete: true,
		MaxSelect: 1, CollectionId: patients.Id,
	})
	collection.Fields.Add(&core.TextField{Name: procedureFieldName, Required: true, Min: 2, Max: procedureNameMax})
	collection.Fields.Add(&core.SelectField{
		Name: procedureFieldType, MaxSelect: 1, Values: enumValues(clinical.ProcedureTypes()),
	})
	collection.Fields.Add(&core.TextField{Name: procedureFieldCode, Max: procedureCodeMax})
	collection.Fields.Add(&core.TextField{Name: procedureFieldDescription, Max: procedureDescriptionMax})
	collection.Fields.Add(&core.DateField{Name: procedureFieldOccurredOn, Required: true})
	collection.Fields.Add(&core.SelectField{
		Name: procedureFieldStatus, Required: true, MaxSelect: 1, Values: enumValues(clinical.OrderStatuses()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: procedureFieldOutcome, MaxSelect: 1, Values: enumValues(clinical.ProcedureOutcomes()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: procedureFieldSetting, MaxSelect: 1, Values: enumValues(clinical.ProcedureSettings()),
	})
	collection.Fields.Add(&core.TextField{Name: procedureFieldComplications, Max: procedureComplicationsMax})
	collection.Fields.Add(&core.NumberField{Name: procedureFieldDurationMin, Min: types.Pointer(0.0)})
	collection.Fields.Add(&core.SelectField{
		Name: procedureFieldAnesthesia, MaxSelect: 1, Values: enumValues(clinical.Anesthesias()),
	})
	collection.Fields.Add(&core.TextField{Name: procedureFieldAnesthesiaNotes, Max: procedureAnesthesiaNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name: procedureFieldPractitioner, MaxSelect: 1, CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: procedureFieldFacility, MaxSelect: 1, CollectionId: facilities.Id,
	})
	collection.Fields.Add(&core.TextField{Name: procedureFieldNotes, Max: procedureNotesMax})
	collection.Fields.Add(&core.RelationField{Name: procedureFieldTags, MaxSelect: 0, CollectionId: tags.Id})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	collection.AddIndex("idx_"+name+"_patient_date", false,
		procedureFieldPatient+", "+procedureFieldOccurredOn+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_patient_status", false,
		procedureFieldPatient+", "+procedureFieldStatus, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func proceduresDown(app core.App) error {
	return deleteCollection(app, kind.Procedure.Collection())
}
