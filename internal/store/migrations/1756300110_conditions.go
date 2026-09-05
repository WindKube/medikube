package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	conditionFieldPatient      = "patient"
	conditionFieldDiagnosis    = "diagnosis"
	conditionFieldStatus       = "status"
	conditionFieldSeverity     = "severity"
	conditionFieldOnsetOn      = "onset_on"
	conditionFieldResolvedOn   = "resolved_on"
	conditionFieldICD10Code    = "icd10_code"
	conditionFieldSNOMEDCode   = "snomed_code"
	conditionFieldPractitioner = "practitioner"
	conditionFieldNotes        = "notes"
	conditionFieldTags         = "tags"
)

const (
	conditionDiagnosisMin  = 2
	conditionDiagnosisMax  = 500
	conditionICD10CodeMax  = 10
	conditionSNOMEDCodeMax = 20
	conditionNotesMax      = 5000
)

func init() {
	register(conditionsUp, conditionsDown)
}

// conditionsUp is data-model §4.2. The medications relation arrives in US6's
// migration; the resolved-requires-resolved_on and ordering rules are the
// domain's (clinical.Condition.Validate), not this schema's.
func conditionsUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	tags, err := app.FindCollectionByNameOrId(TagsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", TagsCollection, err)
	}

	collection := core.NewBaseCollection(kind.Condition.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          conditionFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     conditionFieldDiagnosis,
		Required: true,
		Min:      conditionDiagnosisMin,
		Max:      conditionDiagnosisMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name: conditionFieldStatus, Required: true, MaxSelect: 1,
		Values: enumValues(clinical.ConditionStatuses()),
	})
	collection.Fields.Add(&core.SelectField{
		Name: conditionFieldSeverity, MaxSelect: 1,
		Values: enumValues(clinical.Severities()),
	})
	collection.Fields.Add(&core.DateField{Name: conditionFieldOnsetOn})
	collection.Fields.Add(&core.DateField{Name: conditionFieldResolvedOn})
	collection.Fields.Add(&core.TextField{Name: conditionFieldICD10Code, Max: conditionICD10CodeMax})
	collection.Fields.Add(&core.TextField{Name: conditionFieldSNOMEDCode, Max: conditionSNOMEDCodeMax})
	// Cleared, not cascaded, when the practitioner is deleted: a condition
	// survives the deletion of who diagnosed it (data-model §4.2 note).
	collection.Fields.Add(&core.RelationField{
		Name:          conditionFieldPractitioner,
		Required:      false,
		CascadeDelete: false,
		MaxSelect:     1,
		CollectionId:  practitioners.Id,
	})
	collection.Fields.Add(&core.TextField{Name: conditionFieldNotes, Max: conditionNotesMax})
	collection.Fields.Add(&core.RelationField{
		Name:         conditionFieldTags,
		MaxSelect:    unlimitedTags,
		CollectionId: tags.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Condition.Collection()
	collection.AddIndex("idx_"+name+"_patient_onset", false,
		conditionFieldPatient+", "+conditionFieldOnsetOn+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_patient_status", false,
		conditionFieldPatient+", "+conditionFieldStatus, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func conditionsDown(app core.App) error {
	return deleteCollection(app, kind.Condition.Collection())
}
