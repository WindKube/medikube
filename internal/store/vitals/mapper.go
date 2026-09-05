package vitals

import (
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
	"medikube/internal/store"
)

const (
	fieldPatient        = "patient"
	fieldRecordedAt     = "recorded_at"
	fieldSystolic       = "systolic_mmhg"
	fieldDiastolic      = "diastolic_mmhg"
	fieldHeartRate      = "heart_rate_bpm"
	fieldRespiratory    = "respiratory_rate_bpm"
	fieldTemperature    = "temperature_c"
	fieldSpo2           = "spo2_pct"
	fieldWeight         = "weight_kg"
	fieldHeight         = "height_cm"
	fieldGlucose        = "glucose_mmol_l"
	fieldGlucoseContext = "glucose_context"
	fieldHba1c          = "hba1c_pct"
	fieldPainScale      = "pain_scale"
	fieldDevice         = "device"
	fieldPractitioner   = "practitioner"
	fieldCreated        = "created"
	fieldUpdated        = "updated"
)

var ErrUnexpectedCollection = errors.New("store/measurements: the record is not from this collection")

// FromRecord reads a stored row into the entity. It does not validate.
func FromRecord(record *core.Record) (clinical.Vitals, error) {
	if err := expectCollection(record); err != nil {
		return clinical.Vitals{}, err
	}

	recordedAt, err := recordInstant(record, fieldRecordedAt)
	if err != nil {
		return clinical.Vitals{}, err
	}

	return clinical.Vitals{
		ID:                 record.Id,
		PatientID:          record.GetString(fieldPatient),
		RecordedAt:         recordedAt,
		SystolicMmHg:       recordFloatPtr(record, fieldSystolic),
		DiastolicMmHg:      recordFloatPtr(record, fieldDiastolic),
		HeartRateBpm:       recordFloatPtr(record, fieldHeartRate),
		RespiratoryRateBpm: recordFloatPtr(record, fieldRespiratory),
		TemperatureC:       recordFloatPtr(record, fieldTemperature),
		SpO2Pct:            recordFloatPtr(record, fieldSpo2),
		WeightKg:           recordFloatPtr(record, fieldWeight),
		HeightCm:           recordFloatPtr(record, fieldHeight),
		GlucoseMmolL:       recordFloatPtr(record, fieldGlucose),
		GlucoseContext:     clinical.GlucoseContext(record.GetString(fieldGlucoseContext)),
		Hba1cPct:           recordFloatPtr(record, fieldHba1c),
		PainScale:          recordFloatPtr(record, fieldPainScale),
		Device:             record.GetString(fieldDevice),
		PractitionerID:     record.GetString(fieldPractitioner),
		CreatedAt:          record.GetDateTime(fieldCreated).Time().UTC().Truncate(time.Millisecond),
		UpdatedAt:          record.GetDateTime(fieldUpdated).Time().UTC().Truncate(time.Millisecond),
		Version:            store.Version(record),
	}, nil
}

// ToRecord writes the entity's own columns onto the record. bmi is never
// written — it is derived at render, never stored (FR-037).
func ToRecord(record *core.Record, v clinical.Vitals) error {
	if err := expectCollection(record); err != nil {
		return err
	}

	record.Set(fieldPatient, v.PatientID)
	record.Set(fieldRecordedAt, v.RecordedAt.Time())
	setFloatPtr(record, fieldSystolic, v.SystolicMmHg)
	setFloatPtr(record, fieldDiastolic, v.DiastolicMmHg)
	setFloatPtr(record, fieldHeartRate, v.HeartRateBpm)
	setFloatPtr(record, fieldRespiratory, v.RespiratoryRateBpm)
	setFloatPtr(record, fieldTemperature, v.TemperatureC)
	setFloatPtr(record, fieldSpo2, v.SpO2Pct)
	setFloatPtr(record, fieldWeight, v.WeightKg)
	setFloatPtr(record, fieldHeight, v.HeightCm)
	setFloatPtr(record, fieldGlucose, v.GlucoseMmolL)
	record.Set(fieldGlucoseContext, string(v.GlucoseContext))
	setFloatPtr(record, fieldHba1c, v.Hba1cPct)
	setFloatPtr(record, fieldPainScale, v.PainScale)
	record.Set(fieldDevice, v.Device)
	record.Set(fieldPractitioner, v.PractitionerID)

	return nil
}

// recordFloatPtr and setFloatPtr treat 0 as "not recorded" — see
// internal/store/symptom/mapper.go's identical note: PocketBase's NumberField
// is `NOT NULL DEFAULT 0` and cannot store a true absence.
func recordFloatPtr(record *core.Record, field string) *float64 {
	v := record.GetFloat(field)
	if v == 0 {
		return nil
	}

	return &v
}

func setFloatPtr(record *core.Record, field string, value *float64) {
	if value == nil {
		record.Set(field, 0)

		return
	}

	record.Set(field, *value)
}

func recordInstant(record *core.Record, field string) (clinical.Instant, error) {
	stored := record.GetDateTime(field)
	if stored.IsZero() {
		return clinical.Instant{}, nil
	}

	return clinical.NewInstant(stored.Time().UTC()), nil
}

func expectCollection(record *core.Record) error {
	collection := record.Collection()
	if collection == nil || collection.Name != kind.Vitals.Collection() {
		return fmt.Errorf("%w", ErrUnexpectedCollection)
	}

	return nil
}
