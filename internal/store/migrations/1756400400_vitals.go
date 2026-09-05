package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

const (
	vitalsFieldPatient         = "patient"
	vitalsFieldRecordedAt      = "recorded_at"
	vitalsFieldSystolicMmHg    = "systolic_mmhg"
	vitalsFieldDiastolicMmHg   = "diastolic_mmhg"
	vitalsFieldHeartRateBpm    = "heart_rate_bpm"
	vitalsFieldRespiratoryRate = "respiratory_rate_bpm"
	vitalsFieldTemperatureC    = "temperature_c"
	vitalsFieldSpo2Pct         = "spo2_pct"
	vitalsFieldWeightKg        = "weight_kg"
	vitalsFieldHeightCm        = "height_cm"
	vitalsFieldGlucoseMmolL    = "glucose_mmol_l"
	vitalsFieldGlucoseContext  = "glucose_context"
	vitalsFieldHba1cPct        = "hba1c_pct"
	vitalsFieldPainScale       = "pain_scale"
	vitalsFieldDevice          = "device"
	vitalsFieldPractitioner    = "practitioner"
	vitalsDeviceMax            = 120
)

func init() {
	register(vitalsUp, vitalsDown)
}

// bounded is data-model §4.7's eleven documented ranges (research D-15),
// checked again here as a belt-and-suspenders backstop over
// clinical.Vitals.Validate, the actual authority.
type bounded struct {
	name     string
	min, max float64
}

func vitalsUp(app core.App) error {
	patients, err := app.FindCollectionByNameOrId(patientsCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", patientsCollection, err)
	}

	practitioners, err := app.FindCollectionByNameOrId(practitionersCollection)
	if err != nil {
		return fmt.Errorf("finding %s: %w", practitionersCollection, err)
	}

	collection := core.NewBaseCollection(kind.Vitals.Collection())
	lockRules(collection)

	collection.Fields.Add(&core.RelationField{
		Name:          vitalsFieldPatient,
		Required:      true,
		CascadeDelete: true,
		MaxSelect:     1,
		CollectionId:  patients.Id,
	})
	collection.Fields.Add(&core.DateField{Name: vitalsFieldRecordedAt, Required: true})

	for _, b := range []bounded{
		{vitalsFieldSystolicMmHg, 40, 300},
		{vitalsFieldDiastolicMmHg, 20, 200},
		{vitalsFieldHeartRateBpm, 20, 300},
		{vitalsFieldRespiratoryRate, 4, 80},
		{vitalsFieldTemperatureC, 25, 45},
		{vitalsFieldSpo2Pct, 50, 100},
		{vitalsFieldWeightKg, 0.5, 450},
		{vitalsFieldHeightCm, 30, 272},
		{vitalsFieldGlucoseMmolL, 0.5, 60},
		{vitalsFieldHba1cPct, 2, 20},
		{vitalsFieldPainScale, 0, 10},
	} {
		collection.Fields.Add(&core.NumberField{Name: b.name, Min: ptr(b.min), Max: ptr(b.max)})
	}

	collection.Fields.Add(&core.SelectField{
		Name:      vitalsFieldGlucoseContext,
		MaxSelect: 1,
		Values:    enumValues(clinical.GlucoseContexts()),
	})
	collection.Fields.Add(&core.TextField{Name: vitalsFieldDevice, Max: vitalsDeviceMax})
	collection.Fields.Add(&core.RelationField{
		Name:         vitalsFieldPractitioner,
		MaxSelect:    1,
		CollectionId: practitioners.Id,
	})
	collection.Fields.Add(&core.AutodateField{Name: fieldCreated, OnCreate: true})
	collection.Fields.Add(&core.AutodateField{Name: fieldUpdated, OnCreate: true, OnUpdate: true})

	name := kind.Vitals.Collection()
	collection.AddIndex("idx_"+name+"_patient_at", false,
		vitalsFieldPatient+", "+vitalsFieldRecordedAt+" DESC, id DESC", "")

	if err := app.Save(collection); err != nil {
		return fmt.Errorf("creating %s: %w", name, err)
	}

	return nil
}

func vitalsDown(app core.App) error {
	return deleteCollection(app, kind.Vitals.Collection())
}
