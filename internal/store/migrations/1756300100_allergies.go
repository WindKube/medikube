package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	allergyFieldPatient  = "patient"
	allergyFieldAllergen = "allergen"
	allergyFieldReaction = "reaction"
	allergyFieldSeverity = "severity"
	allergyFieldStatus   = "status"
	allergyFieldOnsetOn  = "onset_on"
	allergyFieldNotes    = "notes"
	allergyFieldTags     = "tags"
)

const (
	allergyAllergenMin = 2
	allergyAllergenMax = 200
	allergyReactionMax = 500
	allergyNotesMax    = 5000
)

func init() {
	register(allergiesUp, allergiesDown)
}

// allergiesUp is data-model §4.1: allergen, severity and status, onset,
// reaction and notes, plus the universal patient and tags fields (§0). The
// medications relation arrives in US6's migration (data-model §4.1 note).
func allergiesUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(kind.Allergy.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          allergyFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     allergyFieldAllergen,
		Required: true,
		Min:      allergyAllergenMin,
		Max:      allergyAllergenMax,
	})
	collection.Fields.Add(&core.TextField{Name: allergyFieldReaction, Max: allergyReactionMax})
	collection.Fields.Add(&core.SelectField{
		Name: allergyFieldSeverity, Required: true, MaxSelect: 1,
		Values: enumValues(clinical.Severities()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: allergyFieldStatus, Required: true, MaxSelect: 1,
		Values: enumValues(clinical.ConditionStatuses()),
	})
	collection.Fields.Add(&core.DateField{Name: allergyFieldOnsetOn})
	collection.Fields.Add(&core.TextField{Name: allergyFieldNotes, Max: allergyNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name:         allergyFieldTags,
		MaxSelect:    0,
		CollectionId: tags.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Allergy.Collection()
	collection.AddIndex("idx_"+name+"_patient_onset", false,
		allergyFieldPatient+", "+allergyFieldOnsetOn+" DESC, id DESC", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func allergiesDown(app core.App) error {
	return deleteCollection(app, kind.Allergy.Collection())
}
