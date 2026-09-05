package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// data-model §4.8's column bounds, the same numbers clinical.Immunization.
// Validate enforces.
const (
	immunizationVaccineNameMin  = 2
	immunizationVaccineNameMax  = 200
	immunizationTradeNameMax    = 200
	immunizationLotNumberMax    = 50
	immunizationManufacturerMax = 200
)

const (
	immunizationFieldPatient      = "patient"
	immunizationFieldPractitioner = "practitioner"
	immunizationFieldFacility     = "facility"
	immunizationFieldVaccineName  = "vaccine_name"
	immunizationFieldTradeName    = "trade_name"
	immunizationFieldAdministered = "administered_on"
	immunizationFieldDoseNumber   = "dose_number"
	immunizationFieldLotNumber    = "lot_number"
	immunizationFieldManufacturer = "manufacturer"
	immunizationFieldSite         = "site"
	immunizationFieldRoute        = "route"
	immunizationFieldExpiresOn    = "expires_on"
	immunizationFieldTags         = "tags"
)

func init() {
	register(immunizationsUp, immunizationsDown)
}

func immunizationsUp(app core.App) error {
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

	collection := core.NewBaseCollection(kind.Immunization.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          immunizationFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: immunizationFieldPractitioner, MaxSelect: 1, CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.RelationField{
		Name: immunizationFieldFacility, MaxSelect: 1, CollectionId: facilities.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name: immunizationFieldVaccineName, Required: true,
		Min: immunizationVaccineNameMin, Max: immunizationVaccineNameMax,
	})
	collection.Fields.Add(&core.TextField{Name: immunizationFieldTradeName, Max: immunizationTradeNameMax})
	collection.Fields.Add(&core.DateField{Name: immunizationFieldAdministered, Required: true})
	collection.Fields.Add(&core.NumberField{Name: immunizationFieldDoseNumber})
	collection.Fields.Add(&core.TextField{Name: immunizationFieldLotNumber, Max: immunizationLotNumberMax})
	collection.Fields.Add(&core.TextField{Name: immunizationFieldManufacturer, Max: immunizationManufacturerMax})
	collection.Fields.Add(&core.SelectField{
		Name: immunizationFieldSite, MaxSelect: 1, Values: enumValues(clinical.ImmunizationSites()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: immunizationFieldRoute, MaxSelect: 1, Values: enumValues(clinical.ImmunizationRoutes()),
	})
	collection.Fields.Add(&core.DateField{Name: immunizationFieldExpiresOn})
	collection.Fields.Add(&core.RelationField{Name: immunizationFieldTags, MaxSelect: 0, CollectionId: tags.Id})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Immunization.Collection()
	collection.AddIndex("idx_"+name+"_patient_date", false,
		immunizationFieldPatient+", "+immunizationFieldAdministered+" DESC, id DESC", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func immunizationsDown(app core.App) error {
	return deleteCollection(app, kind.Immunization.Collection())
}
