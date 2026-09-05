package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// data-model §4.6's column bounds, checked again here (belt and suspenders):
// clinical.Symptom.Validate is the authority, this is what stops a row from
// entering through any other door.
const (
	symptomNameMin      = 1
	symptomNameMax      = 200
	symptomBodySiteMax  = 120
	symptomPainScaleMin = 0
	symptomPainScaleMax = 10
	symptomDurationMin  = 0
	symptomTriggersMax  = 20 * 80 // dbx JSONField has no per-entry bound; enforced in the domain
)

const (
	symptomFieldPatient         = "patient"
	symptomFieldName            = "name"
	symptomFieldCategory        = "category"
	symptomFieldSeverity        = "severity"
	symptomFieldOccurredAt      = "occurred_at"
	symptomFieldDurationMinutes = "duration_minutes"
	symptomFieldPainScale       = "pain_scale"
	symptomFieldBodySite        = "body_site"
	symptomFieldTriggers        = "triggers"
	symptomFieldReliefMethods   = "relief_methods"
	symptomFieldImpact          = "impact"
	symptomFieldResolvedAt      = "resolved_at"
	symptomFieldIsChronic       = "is_chronic"
	symptomFieldStatus          = "status"
)

func init() {
	register(symptomsUp, symptomsDown)
}

func symptomsUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	collection := core.NewBaseCollection(kind.Symptom.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          symptomFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.TextField{
		Name:     symptomFieldName,
		Required: true,
		Min:      symptomNameMin,
		Max:      symptomNameMax,
	})
	collection.Fields.Add(&core.SelectField{
		Name:      symptomFieldCategory,
		MaxSelect: 1,
		Values:    enumValues(clinical.SymptomCategories()),
	})
	collection.Fields.Add(&core.SelectField{
		Name:      symptomFieldSeverity,
		Required:  true,
		MaxSelect: 1,
		Values:    enumValues(clinical.Severities()),
	})
	collection.Fields.Add(&core.DateField{Name: symptomFieldOccurredAt, Required: true})
	collection.Fields.Add(&core.NumberField{
		Name: symptomFieldDurationMinutes,
		Min:  ptr(float64(symptomDurationMin)),
	})
	collection.Fields.Add(&core.NumberField{
		Name: symptomFieldPainScale,
		Min:  ptr(float64(symptomPainScaleMin)),
		Max:  ptr(float64(symptomPainScaleMax)),
	})
	collection.Fields.Add(&core.TextField{Name: symptomFieldBodySite, Max: symptomBodySiteMax})
	collection.Fields.Add(&core.JSONField{Name: symptomFieldTriggers, MaxSize: symptomTriggersMax})
	collection.Fields.Add(&core.JSONField{Name: symptomFieldReliefMethods, MaxSize: symptomTriggersMax})
	collection.Fields.Add(&core.SelectField{
		Name:      symptomFieldImpact,
		MaxSelect: 1,
		Values:    enumValues(clinical.SymptomImpacts()),
	})
	collection.Fields.Add(&core.DateField{Name: symptomFieldResolvedAt})
	collection.Fields.Add(&core.BoolField{Name: symptomFieldIsChronic})
	collection.Fields.Add(&core.SelectField{
		Name:      symptomFieldStatus,
		MaxSelect: 1,
		Values:    enumValues(clinical.ConditionStatuses()),
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Symptom.Collection()
	collection.AddIndex("idx_"+name+"_patient_at", false,
		symptomFieldPatient+", "+symptomFieldOccurredAt+" DESC, id DESC", "")
	collection.AddIndex("idx_"+name+"_patient_name", false,
		symptomFieldPatient+", LOWER("+symptomFieldName+"), "+symptomFieldOccurredAt, "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func symptomsDown(app core.App) error {
	return deleteCollection(app, kind.Symptom.Collection())
}

func ptr(v float64) *float64 { return &v }
