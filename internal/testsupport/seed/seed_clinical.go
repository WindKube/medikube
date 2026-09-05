package seed

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"medikube/internal/domain/clinical"
	"medikube/internal/domain/kind"
)

// The symptom episode ids. Four episodes of the same name (FR-030, FR-031) so
// the derived episode_count and last_occurred_at have something to aggregate.
const (
	SymptomHeadacheOne   = "mksympamara0001"
	SymptomHeadacheTwo   = "mksympamara0002"
	SymptomHeadacheThree = "mksympamara0003"
	SymptomHeadacheFour  = "mksympamara0004"
)

// The vitals measurement set ids, spanning two months.
const (
	VitalsOne   = "mkvitlamara0001"
	VitalsTwo   = "mkvitlamara0002"
	VitalsThree = "mkvitlamara0003"
	VitalsFour  = "mkvitlamara0004"
	VitalsFive  = "mkvitlamara0005"
	VitalsSix   = "mkvitlamara0006"
)

// Symptoms is four recordings of the same name for account A's self patient,
// which is FR-030/FR-031's aggregate case.
func Symptoms() []clinical.Symptom {
	return []clinical.Symptom{
		{
			ID: SymptomHeadacheOne, PatientID: accountAPatientSelfID,
			Name: "Headache", Severity: clinical.SeverityMild,
			OccurredAt: mustInstant("2025-06-01T08:00:00Z"),
		},
		{
			ID: SymptomHeadacheTwo, PatientID: accountAPatientSelfID,
			Name: "Headache", Severity: clinical.SeverityModerate,
			OccurredAt:      mustInstant("2025-06-10T09:30:00Z"),
			DurationMinutes: intPtr(45),
		},
		{
			ID: SymptomHeadacheThree, PatientID: accountAPatientSelfID,
			Name: "headache", Severity: clinical.SeverityMild,
			OccurredAt: mustInstant("2025-07-02T20:00:00Z"),
		},
		{
			ID: SymptomHeadacheFour, PatientID: accountAPatientSelfID,
			Name: "HEADACHE", Severity: clinical.SeveritySevere,
			OccurredAt: mustInstant("2025-07-15T14:00:00Z"),
			PainScale:  intPtr(8),
			IsChronic:  true,
			Status:     clinical.ConditionStatusActive,
		},
	}
}

func mustInstant(raw string) clinical.Instant {
	var parsed clinical.Instant
	if err := parsed.UnmarshalText([]byte(raw)); err != nil {
		panic(fmt.Sprintf("seed: %q is not an instant: %v", raw, err))
	}

	return parsed
}

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }

// Vitals is six measurement sets for account A's self patient, spanning two
// months.
func Vitals() []clinical.Vitals {
	return []clinical.Vitals{
		{
			ID: VitalsOne, PatientID: accountAPatientSelfID,
			RecordedAt:   mustInstant("2025-06-01T07:00:00Z"),
			SystolicMmHg: floatPtr(120), DiastolicMmHg: floatPtr(80),
			HeartRateBpm: floatPtr(68), WeightKg: floatPtr(70), HeightCm: floatPtr(175),
		},
		{
			ID: VitalsTwo, PatientID: accountAPatientSelfID,
			RecordedAt:   mustInstant("2025-06-10T07:00:00Z"),
			SystolicMmHg: floatPtr(122), DiastolicMmHg: floatPtr(78),
			TemperatureC: floatPtr(36.8),
		},
		{
			ID: VitalsThree, PatientID: accountAPatientSelfID,
			RecordedAt:   mustInstant("2025-06-20T07:00:00Z"),
			GlucoseMmolL: floatPtr(5.4), GlucoseContext: clinical.GlucoseContextFasting,
		},
		{
			ID: VitalsFour, PatientID: accountAPatientSelfID,
			RecordedAt: mustInstant("2025-07-01T07:00:00Z"),
			WeightKg:   floatPtr(69.5), HeightCm: floatPtr(175),
		},
		{
			ID: VitalsFive, PatientID: accountAPatientSelfID,
			RecordedAt: mustInstant("2025-07-15T07:00:00Z"),
			SpO2Pct:    floatPtr(98), RespiratoryRateBpm: floatPtr(16),
		},
		{
			ID: VitalsSix, PatientID: accountAPatientSelfID,
			RecordedAt: mustInstant("2025-07-30T07:00:00Z"),
			Hba1cPct:   floatPtr(5.6), PainScale: floatPtr(2),
			Device: "home BP monitor",
		},
	}
}

const (
	columnSeverity        = "severity"
	columnOccurredAt      = "occurred_at"
	columnDurationMinutes = "duration_minutes"
	columnPainScale       = "pain_scale"
	columnIsChronic       = "is_chronic"

	columnRecordedAt         = "recorded_at"
	columnSystolicMmHg       = "systolic_mmhg"
	columnDiastolicMmHg      = "diastolic_mmhg"
	columnHeartRateBpm       = "heart_rate_bpm"
	columnRespiratoryRateBpm = "respiratory_rate_bpm"
	columnTemperatureC       = "temperature_c"
	columnSpo2Pct            = "spo2_pct"
	columnWeightKg           = "weight_kg"
	columnHeightCm           = "height_cm"
	columnGlucoseMmolL       = "glucose_mmol_l"
	columnGlucoseContext     = "glucose_context"
	columnHba1cPct           = "hba1c_pct"
	columnDevice             = "device"
)

func applySymptoms(app core.App) error {
	name := kind.Symptom.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnName, columnSeverity, columnOccurredAt,
		columnDurationMinutes, columnPainScale, columnIsChronic, columnStatus,
	); err != nil {
		return err
	}

	for _, symptom := range Symptoms() {
		if err := symptom.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", symptom.ID, err)
		}

		record, err := findOrNew(app, collection, symptom.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, symptom.PatientID)
		record.Set(columnName, symptom.Name)
		record.Set(columnSeverity, string(symptom.Severity))
		record.Set(columnOccurredAt, symptom.OccurredAt.String())
		record.Set(columnDurationMinutes, symptom.DurationMinutes)
		record.Set(columnPainScale, symptom.PainScale)
		record.Set(columnIsChronic, symptom.IsChronic)
		record.Set(columnStatus, string(symptom.Status))

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", symptom.ID, err)
		}
	}

	return nil
}

func applyVitals(app core.App) error {
	name := kind.Vitals.Collection()

	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return fmt.Errorf("finding %s: %w", name, err)
	}

	if err := requireColumns(collection,
		columnPatient, columnRecordedAt, columnSystolicMmHg, columnDiastolicMmHg,
		columnHeartRateBpm, columnRespiratoryRateBpm, columnTemperatureC, columnSpo2Pct,
		columnWeightKg, columnHeightCm, columnGlucoseMmolL, columnGlucoseContext,
		columnHba1cPct, columnPainScale, columnDevice,
	); err != nil {
		return err
	}

	for _, v := range Vitals() {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("seeding %s: %w", v.ID, err)
		}

		record, err := findOrNew(app, collection, v.ID)
		if err != nil {
			return err
		}

		record.Set(columnPatient, v.PatientID)
		record.Set(columnRecordedAt, v.RecordedAt.String())
		record.Set(columnSystolicMmHg, v.SystolicMmHg)
		record.Set(columnDiastolicMmHg, v.DiastolicMmHg)
		record.Set(columnHeartRateBpm, v.HeartRateBpm)
		record.Set(columnRespiratoryRateBpm, v.RespiratoryRateBpm)
		record.Set(columnTemperatureC, v.TemperatureC)
		record.Set(columnSpo2Pct, v.SpO2Pct)
		record.Set(columnWeightKg, v.WeightKg)
		record.Set(columnHeightCm, v.HeightCm)
		record.Set(columnGlucoseMmolL, v.GlucoseMmolL)
		record.Set(columnGlucoseContext, string(v.GlucoseContext))
		record.Set(columnHba1cPct, v.Hba1cPct)
		record.Set(columnPainScale, v.PainScale)
		record.Set(columnDevice, v.Device)

		if err := app.Save(record); err != nil {
			return fmt.Errorf("seeding %s: %w", v.ID, err)
		}
	}

	return nil
}
