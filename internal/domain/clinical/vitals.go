package clinical

import (
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"medikube/internal/domain"
)

// Vitals is one measurement set (FR-033), storage is SI throughout and
// conversion happens at the presentation edge (research D-15). Every field but
// recorded_at is a pointer: SQLite's own bounds never admit zero, but a nil
// pointer and a zero measurement must not be confused, and FR-034's
// at-least-one rule needs to tell "not taken" from "taken as zero".
type Vitals struct {
	ID        string
	PatientID string

	RecordedAt Instant

	SystolicMmHg       *float64
	DiastolicMmHg      *float64
	HeartRateBpm       *float64
	RespiratoryRateBpm *float64
	TemperatureC       *float64
	SpO2Pct            *float64
	WeightKg           *float64
	HeightCm           *float64
	GlucoseMmolL       *float64
	GlucoseContext     GlucoseContext
	Hba1cPct           *float64
	PainScale          *float64

	Device         string
	PractitionerID string

	// Tags is data-model §0.8's universal field: any number of the owning
	// account's tags, applied with replace-set semantics (FR-064).
	Tags []string

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   string
}

// CodeAtLeastOneMeasurement is FR-034's refusal: a set with no measurement at
// all is not a reading.
const CodeAtLeastOneMeasurement = "at_least_one_measurement"

const vitalsDeviceMax = 120

// bound is one of data-model §4.7's eleven documented ranges (research D-15).
type bound struct {
	field    string
	value    *float64
	min, max float64
}

// BMI is the derived value FR-037 requires and never stores. ok is false when
// either input is missing, which is what an unrecorded height or weight is.
func (v Vitals) BMI() (bmi float64, ok bool) {
	if v.WeightKg == nil || v.HeightCm == nil {
		return 0, false
	}

	return BMI(*v.WeightKg, *v.HeightCm), true
}

// Validate reports every offending field at once (FR-027), in data-model
// §4.7's column order.
func (v Vitals) Validate() error {
	var invalid domain.ValidationError

	if v.RecordedAt.IsZero() {
		invalid.Add("recorded_at", domain.CodeRequired, "when it was recorded is required")
	} else if v.RecordedAt.After(Now()) {
		invalid.Add("recorded_at", CodeNotFuture, "recorded_at cannot be in the future")
	}

	bounds := []bound{
		{"systolic_mmhg", v.SystolicMmHg, 40, 300},
		{"diastolic_mmhg", v.DiastolicMmHg, 20, 200},
		{"heart_rate_bpm", v.HeartRateBpm, 20, 300},
		{"respiratory_rate_bpm", v.RespiratoryRateBpm, 4, 80},
		{"temperature_c", v.TemperatureC, 25, 45},
		{"spo2_pct", v.SpO2Pct, 50, 100},
		{"weight_kg", v.WeightKg, 0.5, 450},
		{"height_cm", v.HeightCm, 30, 272},
		{"glucose_mmol_l", v.GlucoseMmolL, 0.5, 60},
		{"hba1c_pct", v.Hba1cPct, 2, 20},
		{"pain_scale", v.PainScale, 0, 10},
	}

	present := false

	for _, b := range bounds {
		if b.value == nil {
			continue
		}

		present = true

		if *b.value < b.min || *b.value > b.max {
			invalid.Addf(b.field, domain.CodeOutOfRange, "%s accepts %g to %g", b.field, b.min, b.max)
		}
	}

	if !present {
		invalid.Add("measurements", CodeAtLeastOneMeasurement, "at least one measurement is required")
	}

	switch {
	case v.SystolicMmHg != nil && v.DiastolicMmHg == nil:
		invalid.Add("diastolic_mmhg", domain.CodeRequired, "diastolic is required when systolic is given")
	case v.DiastolicMmHg != nil && v.SystolicMmHg == nil:
		invalid.Add("systolic_mmhg", domain.CodeRequired, "systolic is required when diastolic is given")
	case v.SystolicMmHg != nil && v.DiastolicMmHg != nil && *v.DiastolicMmHg >= *v.SystolicMmHg:
		invalid.Add("diastolic_mmhg", domain.CodeInvalidValue, "diastolic must be less than systolic")
	}

	if v.GlucoseContext != "" && !v.GlucoseContext.Valid() {
		invalid.Add("glucose_context", domain.CodeInvalidValue, "not one of the contexts MediKube accepts")
	}

	if utf8.RuneCountInString(v.Device) > vitalsDeviceMax {
		invalid.Addf("device", domain.CodeTooLong, "the device accepts at most %d characters", vitalsDeviceMax)
	}

	return invalid.OrNil()
}

// MarshalZerologObject emits the two identifiers and nothing else — every
// measurement here is PHI (constitution VII).
func (v Vitals) MarshalZerologObject(e *zerolog.Event) {
	e.Str("measurement_set_id", v.ID).Str("patient_id", v.PatientID)
}
